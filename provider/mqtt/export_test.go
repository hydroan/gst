package mqtt

// Close exposes the unexported lifecycle exit to the external test package;
// production code reaches it only through the provider registry.
var Close = closeProvider
