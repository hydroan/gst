package ldap

import (
	"context"
	"crypto/tls"
	"fmt"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/go-ldap/ldap/v3"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap"
)

var (
	initialized bool
	gconn       *ldap.Conn
	mu          sync.RWMutex
	heartbeat   *time.Ticker
)

// Init initializes the global LDAP connection.
// It reads LDAP configuration from config.App.Ldap.
// If LDAP is not enabled, it returns nil.
// The function is thread-safe and ensures the connection is initialized only once.
func Init() (err error) {
	cfg := config.App.Ldap
	if !cfg.Enabled {
		return nil
	}
	mu.Lock()
	defer mu.Unlock()
	if initialized {
		return nil
	}

	if gconn, err = New(cfg); err != nil {
		return errors.Wrap(err, "failed to connect to LDAP server")
	}

	// Test the connection by performing a simple search
	searchRequest := ldap.NewSearchRequest(
		cfg.BaseDN,
		ldap.ScopeBaseObject,
		ldap.NeverDerefAliases,
		1,
		int(cfg.RequestTimeout.Seconds()),
		false,
		"(objectClass=*)",
		[]string{"1.1"},
		nil,
	)

	if _, err = gconn.Search(searchRequest); err != nil {
		gconn.Close()
		gconn = nil
		return errors.Wrap(err, "failed to connect to LDAP server")
	}

	// Start the heartbeat ticker to check connection health
	if cfg.Heartbeat > 0 {
		heartbeat = time.NewTicker(cfg.Heartbeat)
		go func() {
			for range heartbeat.C {
				checkConnection()
			}
		}()
	}

	zap.S().Infow("successfully connected to LDAP server", "host", cfg.Host, "port", cfg.Port)

	initialized = true
	return nil
}

// New returns a new LDAP connection with given configuration.
// It's the caller's responsibility to close the connection,
// caller should always call Close() when it's no longer needed.
func New(cfg config.Ldap) (*ldap.Conn, error) {
	if cfg.Host == "" {
		return nil, errors.New("no LDAP host provided")
	}

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	var conn *ldap.Conn
	var err error

	if cfg.TLSEnabled {
		var tlsConf *tls.Config
		if tlsConf, err = util.BuildTLSConfig(cfg.CertFile, cfg.KeyFile, cfg.CAFile, cfg.InsecureSkipVerify); err != nil {
			return nil, errors.Wrap(err, "failed to build TLS config")
		}
		if conn, err = ldap.DialURL("ldaps://"+addr, ldap.DialWithTLSConfig(tlsConf)); err != nil {
			return nil, errors.Wrap(err, "failed to connect to LDAP server with TLS")
		}
	} else {
		if conn, err = ldap.DialURL("ldap://" + addr); err != nil {
			return nil, errors.Wrap(err, "failed to connect to LDAP server")
		}
	}

	if cfg.RequestTimeout > 0 {
		conn.SetTimeout(cfg.RequestTimeout)
	}
	if cfg.BindDN != "" && cfg.BindPassword != "" {
		if err = conn.Bind(cfg.BindDN, cfg.BindPassword); err != nil {
			conn.Close()
			return nil, errors.Wrap(err, "failed to bind with the service account")
		}
	}
	return conn, nil
}

// Conn returns the global LDAP connection.
// It returns nil if the connection is not initialized.
func Conn() *ldap.Conn {
	mu.RLock()
	defer mu.RUnlock()
	return gconn
}

// Close closes the global LDAP connection.
func Close() {
	mu.Lock()
	defer mu.Unlock()

	// Stop the heartbeat ticker
	if heartbeat != nil {
		heartbeat.Stop()
		heartbeat = nil
	}

	if gconn != nil {
		if err := gconn.Close(); err != nil {
			zap.S().Errorw("failed to close LDAP connection", "error", err)
		} else {
			zap.S().Infow("successfully closed LDAP connection")
		}
		gconn = nil
	}
	initialized = false
}

// checkConnection verifies the LDAP connection is still valid
// and reconnects if necessary.
func checkConnection() {
	if gconn == nil || !initialized {
		return
	}

	cfg := config.App.Ldap

	// Create a simple search request to test the connection
	searchRequest := ldap.NewSearchRequest(
		cfg.BaseDN,
		ldap.ScopeBaseObject,
		ldap.NeverDerefAliases,
		1,
		int(cfg.RequestTimeout.Seconds()),
		false,
		"(objectClass=*)",
		[]string{"1.1"},
		nil,
	)

	// Test the connection
	if _, err := gconn.Search(searchRequest); err != nil {
		zap.S().Warnw("LDAP connection check failed, reconnecting", "error", err)

		// Close the old connection
		gconn.Close()

		// Try to create a new connection
		newClient, err := New(cfg)
		if err != nil {
			zap.S().Errorw("failed to reconnect to LDAP server", "error", err)
			gconn = nil
			return
		}

		gconn = newClient
		zap.S().Infow("successfully reconnected to LDAP server")
	}
}

// Search performs an LDAP search with the given parameters
func Search(baseDN, filter string, attributes []string, scope int) ([]*ldap.Entry, error) {
	conn := Conn()
	if conn == nil {
		return nil, errors.New("LDAP connection not initialized")
	}

	cfg := config.App.Ldap

	searchRequest := ldap.NewSearchRequest(
		baseDN,
		scope,
		cfg.Deref,
		0, // Size limit
		int(cfg.RequestTimeout.Seconds()),
		false, // Types only
		filter,
		attributes,
		nil, // Controls
	)

	var entries []*ldap.Entry

	// If pagination is enabled (page size > 0), use the paged search
	if cfg.PageSize > 0 {
		control := ldap.NewControlPaging(uint32(cfg.PageSize)) //nolint:gosec

		for {
			searchRequest.Controls = []ldap.Control{control}

			response, err := conn.Search(searchRequest)
			if err != nil {
				return nil, errors.Wrap(err, "LDAP search failed")
			}

			entries = append(entries, response.Entries...)

			// Extract the paging control from the response
			pagingResult := ldap.FindControl(response.Controls, ldap.ControlTypePaging)
			if pagingResult == nil {
				break
			}

			// Convert to paging control
			pagingControl, ok := pagingResult.(*ldap.ControlPaging)
			if !ok || len(pagingControl.Cookie) == 0 {
				break
			}

			// Set the cookie for the next request
			control.SetCookie(pagingControl.Cookie)
		}
	} else {
		// Perform a simple search
		response, err := conn.Search(searchRequest)
		if err != nil {
			return nil, errors.Wrap(err, "LDAP search failed")
		}

		entries = response.Entries
	}

	return entries, nil
}

// SearchWithContext performs an LDAP search with context for timeout control
func SearchWithContext(ctx context.Context, baseDN, filter string, attributes []string, scope int) ([]*ldap.Entry, error) {
	// Create a channel to receive the search result
	resultCh := make(chan struct {
		entries []*ldap.Entry
		err     error
	})

	go func() {
		entries, err := Search(baseDN, filter, attributes, scope)
		resultCh <- struct {
			entries []*ldap.Entry
			err     error
		}{entries, err}
	}()

	// Wait for either the context to be done or the search to complete
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-resultCh:
		return result.entries, result.err
	}
}

// Authenticate authenticates a user with the given username and password
func Authenticate(username, password string) (bool, error) {
	conn := Conn()
	if conn == nil {
		return false, errors.New("LDAP connection not initialized")
	}

	cfg := config.App.Ldap

	// First, find the user's DN
	filter := fmt.Sprintf("(&%s(%s=%s))", cfg.UserFilter, cfg.UserAttribute, ldap.EscapeFilter(username))

	searchDN := cfg.UserDN
	if searchDN == "" {
		searchDN = cfg.BaseDN
	}

	entries, err := Search(searchDN, filter, []string{"dn"}, cfg.Scope)
	if err != nil {
		return false, errors.Wrap(err, "failed to find user")
	}

	if len(entries) != 1 {
		return false, errors.Errorf("found %d entries for user %s, expected 1", len(entries), username)
	}

	userDN := entries[0].DN

	authConn, err := New(config.App.Ldap)
	if err != nil {
		return false, errors.Wrap(err, "failed to connect to LDAP server")
	}
	defer authConn.Close()

	// Try to bind with the user's credentials
	err = authConn.Bind(userDN, password)
	if err != nil {
		if ldap.IsErrorWithCode(err, ldap.LDAPResultInvalidCredentials) {
			return false, nil // Invalid credentials
		}
		return false, errors.Wrap(err, "authentication failed")
	}

	return true, nil
}

// GetUser returns a user entry for a given username
func GetUser(username string, attributes []string) (*ldap.Entry, error) {
	conn := Conn()
	if conn == nil {
		return nil, errors.New("LDAP connection not initialized")
	}

	cfg := config.App.Ldap

	if len(attributes) == 0 {
		attributes = cfg.Attributes
	}

	filter := fmt.Sprintf("(&%s(%s=%s))", cfg.UserFilter, cfg.UserAttribute, ldap.EscapeFilter(username))

	searchDN := cfg.UserDN
	if searchDN == "" {
		searchDN = cfg.BaseDN
	}

	entries, err := Search(searchDN, filter, attributes, cfg.Scope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to find user")
	}

	if len(entries) != 1 {
		return nil, errors.Errorf("found %d entries for user %s, expected 1", len(entries), username)
	}

	return entries[0], nil
}

// GetGroup returns a group entry for a given group name
func GetGroup(groupName string, attributes []string) (*ldap.Entry, error) {
	conn := Conn()
	if conn == nil {
		return nil, errors.New("LDAP connection not initialized")
	}

	cfg := config.App.Ldap

	if len(attributes) == 0 {
		attributes = cfg.Attributes
	}

	filter := fmt.Sprintf("(&%s(cn=%s))", cfg.GroupFilter, ldap.EscapeFilter(groupName))

	searchDN := cfg.GroupDN
	if searchDN == "" {
		searchDN = cfg.BaseDN
	}

	entries, err := Search(searchDN, filter, attributes, cfg.Scope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to find group")
	}

	if len(entries) != 1 {
		return nil, errors.Errorf("found %d entries for group %s, expected 1", len(entries), groupName)
	}

	return entries[0], nil
}

// GetUserGroups returns all groups for a given username
func GetUserGroups(username string) ([]string, error) {
	conn := Conn()
	if conn == nil {
		return nil, errors.New("LDAP connection not initialized")
	}

	cfg := config.App.Ldap

	// First, find the user's DN
	userFilter := fmt.Sprintf("(&%s(%s=%s))", cfg.UserFilter, cfg.UserAttribute, ldap.EscapeFilter(username))

	searchDN := cfg.UserDN
	if searchDN == "" {
		searchDN = cfg.BaseDN
	}

	entries, err := Search(searchDN, userFilter, []string{"dn"}, cfg.Scope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to find user")
	}

	if len(entries) != 1 {
		return nil, errors.Errorf("found %d entries for user %s, expected 1", len(entries), username)
	}

	userDN := entries[0].DN

	// Now find all groups that have this user as a member
	groupSearchDN := cfg.GroupDN
	if groupSearchDN == "" {
		groupSearchDN = cfg.BaseDN
	}

	groupFilter := fmt.Sprintf("(&%s(%s=%s))", cfg.GroupFilter, cfg.GroupAttribute, ldap.EscapeFilter(userDN))

	groupEntries, err := Search(groupSearchDN, groupFilter, []string{"cn"}, cfg.Scope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to find user groups")
	}

	groups := make([]string, len(groupEntries))
	for i, entry := range groupEntries {
		cn := entry.GetAttributeValue("cn")
		if cn != "" {
			groups[i] = cn
		} else {
			// If cn is not available, use the DN
			groups[i] = entry.DN
		}
	}

	return groups, nil
}

// GetGroupMembers returns all members of a given group
func GetGroupMembers(groupName string) ([]string, error) {
	conn := Conn()
	if conn == nil {
		return nil, errors.New("LDAP connection not initialized")
	}

	cfg := config.App.Ldap

	// First, find the group's entry
	groupEntry, err := GetGroup(groupName, []string{cfg.GroupAttribute})
	if err != nil {
		return nil, err
	}

	// Get the member DNs
	memberDNS := groupEntry.GetAttributeValues(cfg.GroupAttribute)

	// Convert member DNs to usernames
	usernames := make([]string, 0, len(memberDNS))
	for _, memberDN := range memberDNS {
		// Search for this user to get their username
		filter := fmt.Sprintf("(%s)", ldap.EscapeFilter(cfg.UserAttribute))

		entries, err := Search(memberDN, filter, []string{cfg.UserAttribute}, ldap.ScopeBaseObject)
		if err != nil {
			// Skip this user if there's an error
			zap.S().Warnw("failed to find user attributes", "dn", memberDN, "error", err)
			continue
		}

		if len(entries) == 1 {
			username := entries[0].GetAttributeValue(cfg.UserAttribute)
			if username != "" {
				usernames = append(usernames, username)
			}
		}
	}

	return usernames, nil
}

// AddUser adds a new user to the LDAP directory
func AddUser(username, firstName, lastName, email, password string, attributes map[string][]string) error {
	conn := Conn()
	if conn == nil {
		return errors.New("LDAP connection not initialized")
	}

	cfg := config.App.Ldap

	// Construct the new user entry
	userDN := fmt.Sprintf("%s=%s,%s", cfg.UserAttribute, username, cfg.UserDN)

	// Create a list of attributes
	attrs := []*ldap.Attribute{
		{
			Type: "objectClass",
			Vals: []string{"top", "person", "organizationalPerson", "inetOrgPerson"},
		},
		{
			Type: cfg.UserAttribute,
			Vals: []string{username},
		},
		{
			Type: "cn",
			Vals: []string{fmt.Sprintf("%s %s", firstName, lastName)},
		},
		{
			Type: "givenName",
			Vals: []string{firstName},
		},
		{
			Type: "sn",
			Vals: []string{lastName},
		},
	}

	// Add email if provided
	if email != "" {
		attrs = append(attrs, &ldap.Attribute{
			Type: "mail",
			Vals: []string{email},
		})
	}

	// Add additional attributes
	for attrName, attrVals := range attributes {
		attrs = append(attrs, &ldap.Attribute{
			Type: attrName,
			Vals: attrVals,
		})
	}

	// Create the add request
	addReq := ldap.NewAddRequest(userDN, nil)
	for _, attr := range attrs {
		addReq.Attribute(attr.Type, attr.Vals)
	}

	// Add the password if provided
	if password != "" {
		addReq.Attribute("userPassword", []string{password})
	}

	// Execute the add operation
	err := conn.Add(addReq)
	if err != nil {
		return errors.Wrap(err, "failed to add user")
	}

	return nil
}

// ModifyUser modifies an existing user in the LDAP directory
func ModifyUser(username string, changes map[string][]string) error {
	conn := Conn()
	if conn == nil {
		return errors.New("LDAP connection not initialized")
	}

	// First, find the user's DN
	userEntry, err := GetUser(username, []string{"dn"})
	if err != nil {
		return err
	}

	// Create the modify request
	modifyReq := ldap.NewModifyRequest(userEntry.DN, nil)

	// Add modifications
	for attrName, attrVals := range changes {
		modifyReq.Replace(attrName, attrVals)
	}

	// Execute the modify operation
	err = conn.Modify(modifyReq)
	if err != nil {
		return errors.Wrap(err, "failed to modify user")
	}

	return nil
}

// DeleteUser deletes a user from the LDAP directory
func DeleteUser(username string) error {
	conn := Conn()
	if conn == nil {
		return errors.New("LDAP connection not initialized")
	}

	// First, find the user's DN
	userEntry, err := GetUser(username, []string{"dn"})
	if err != nil {
		return err
	}

	// Create the delete request
	delReq := ldap.NewDelRequest(userEntry.DN, nil)

	// Execute the delete operation
	err = conn.Del(delReq)
	if err != nil {
		return errors.Wrap(err, "failed to delete user")
	}

	return nil
}

// AddUserToGroup adds a user to a group
func AddUserToGroup(username, groupName string) error {
	conn := Conn()
	if conn == nil {
		return errors.New("LDAP connection not initialized")
	}

	// Find the user's DN
	userEntry, err := GetUser(username, []string{"dn"})
	if err != nil {
		return err
	}

	// Find the group's DN
	groupEntry, err := GetGroup(groupName, []string{"dn"})
	if err != nil {
		return err
	}

	cfg := config.App.Ldap

	// Create the modify request to add the user to the group
	modifyReq := ldap.NewModifyRequest(groupEntry.DN, nil)
	modifyReq.Add(cfg.GroupAttribute, []string{userEntry.DN})

	// Execute the modify operation
	err = conn.Modify(modifyReq)
	if err != nil {
		return errors.Wrap(err, "failed to add user to group")
	}

	return nil
}

// RemoveUserFromGroup removes a user from a group
func RemoveUserFromGroup(username, groupName string) error {
	conn := Conn()
	if conn == nil {
		return errors.New("LDAP connection not initialized")
	}

	// Find the user's DN
	userEntry, err := GetUser(username, []string{"dn"})
	if err != nil {
		return err
	}

	// Find the group's DN
	groupEntry, err := GetGroup(groupName, []string{"dn"})
	if err != nil {
		return err
	}

	cfg := config.App.Ldap

	// Create the modify request to remove the user from the group
	modifyReq := ldap.NewModifyRequest(groupEntry.DN, nil)
	modifyReq.Delete(cfg.GroupAttribute, []string{userEntry.DN})

	// Execute the modify operation
	err = conn.Modify(modifyReq)
	if err != nil {
		return errors.Wrap(err, "failed to remove user from group")
	}

	return nil
}
