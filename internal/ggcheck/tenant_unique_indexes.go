package ggcheck

import (
	"os"

	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/hydroan/gst/internal/modelinspect"
)

// TenantUniqueIndexes holds the unique indexes of the models embedding
// tenant.Scope to the tenant.
var TenantUniqueIndexes = Check{
	Name: "Tenant unique indexes",
	Rule: "every unique index a model embedding tenant.Scope declares must name TenantID among its fields: the rows of such a model belong to one tenant each, so its uniqueness is declared within the tenant, {Fields: []string{\"TenantID\", \"Code\"}, Unique: true} rather than {Fields: []string{\"Code\"}, Unique: true}",
	run:  checkTenantUniqueIndexes,
}

// checkTenantUniqueIndexes reports the tenant-scoped models declaring a unique
// index without the tenant, which table preparation refuses at startup (see
// modelschema.ValidateTenantIndexes), whatever database the project names.
// Which column carries the tenant is gorm's to decide from the structs a
// model embeds, so the check asks the models themselves through the column
// inspection (see inspectedModelViolations).
func checkTenantUniqueIndexes(ignore gghelper.ProjectIgnore) []string {
	if _, err := os.Stat(ggconst.DirModel); os.IsNotExist(err) {
		return nil
	}
	return inspectedModelViolations(ignore, "tenant unique indexes", func(model modelinspect.ModelColumns) string {
		return model.TenantIndexViolation
	})
}
