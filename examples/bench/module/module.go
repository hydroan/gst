// Package module assembles the application's business modules.
//
// Call each module's Register function in init below: built-in gst modules
// such as github.com/hydroan/gst/module/iam, and your own. For your own
// resources, create one subpackage per resource under module/ and expose a
// Register function that wires model, service and routes via module.Use.
//
// See github.com/hydroan/gst/module/helloworld for a complete example.
package module

func init() {
	// TODO: call your module Register functions here.
}
