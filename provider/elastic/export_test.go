package elastic

// Init exposes the unexported lifecycle entry to the external test package;
// production code reaches it only through the provider registry.
var Init = initProvider
