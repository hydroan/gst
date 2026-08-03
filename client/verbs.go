package client

// The verb functions are the client-side pairing of the DSL's standard and
// custom actions: the path names the route a Design() declared, the payload
// carries the Payload[*XxxReq]() body, and the RSP type parameter is the
// Result[*XxxRsp]() type decoded from the envelope data.
//
// They are package-level generic functions rather than Client methods only
// because Go methods cannot declare their own type parameters.
//
// TODO: fold these verb functions into Client methods (cli.Post[RSP](...))
// once the language supports parameterized methods (expected in go1.27), and
// update the package doc accordingly.

import (
	"encoding/json"
	"net/http"

	"github.com/cockroachdb/errors"
)

// Get sends a GET request and decodes the envelope data into RSP. GET carries
// no request body, matching the framework contract that the List and Get
// actions declare no Payload.
func Get[RSP any](cli *Client, path string, opts ...RequestOption) (*RSP, error) {
	return callVerb[RSP](cli, http.MethodGet, path, nil, opts)
}

// Post sends a POST request and decodes the envelope data into RSP. It pairs
// the DSL Create action and the POST-shaped custom action routes.
func Post[RSP any](cli *Client, path string, payload any, opts ...RequestOption) (*RSP, error) {
	return callVerb[RSP](cli, http.MethodPost, path, payload, opts)
}

// Put sends a PUT request and decodes the envelope data into RSP. It pairs
// the DSL Update action and the standard batch update route.
func Put[RSP any](cli *Client, path string, payload any, opts ...RequestOption) (*RSP, error) {
	return callVerb[RSP](cli, http.MethodPut, path, payload, opts)
}

// Patch sends a PATCH request and decodes the envelope data into RSP. It
// pairs the DSL Patch action and the standard batch patch route.
func Patch[RSP any](cli *Client, path string, payload any, opts ...RequestOption) (*RSP, error) {
	return callVerb[RSP](cli, http.MethodPatch, path, payload, opts)
}

// Delete sends a DELETE request and decodes the envelope data into RSP. It
// pairs the DSL Delete action; the payload covers the standard batch delete
// body, deleting by id passes nil.
func Delete[RSP any](cli *Client, path string, payload any, opts ...RequestOption) (*RSP, error) {
	return callVerb[RSP](cli, http.MethodDelete, path, payload, opts)
}

// callVerb executes the request and decodes the envelope data field into RSP.
// An empty data field decodes to the zero value of RSP.
func callVerb[RSP any](cli *Client, method, path string, payload any, opts []RequestOption) (*RSP, error) {
	resp, err := cli.Do(method, path, payload, opts...)
	if err != nil {
		return nil, err
	}
	rsp := new(RSP)
	if len(resp.Data) > 0 {
		if err := json.Unmarshal(resp.Data, rsp); err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal the response data")
		}
	}
	return rsp, nil
}
