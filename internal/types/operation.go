package types

// Operation is the operation an operation log records, one per action: the
// snake case name of the action, create or patch_many.
type Operation string

// The operations an operation log records.
const (
	OperationCreate Operation = "create"
	OperationDelete Operation = "delete"
	OperationUpdate Operation = "update"
	OperationPatch  Operation = "patch"
	OperationList   Operation = "list"
	OperationGet    Operation = "get"
	OperationExport Operation = "export"
	OperationImport Operation = "import"

	OperationCreateMany Operation = "create_many"
	OperationDeleteMany Operation = "delete_many"
	OperationUpdateMany Operation = "update_many"
	OperationPatchMany  Operation = "patch_many"
)
