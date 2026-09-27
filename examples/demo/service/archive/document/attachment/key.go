package attachment

import "path"

// attachmentKey is the object key the attachment of the document is kept
// under: documents/<document id>/attachment.
func attachmentKey(documentID string) string {
	return path.Join("documents", documentID, "attachment")
}
