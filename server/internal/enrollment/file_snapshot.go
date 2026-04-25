package enrollment

type ManagedFileSnapshot struct {
	FileID           string `json:"fileId"`
	Name             string `json:"name"`
	Path             string `json:"path"`
	DownloadPath     string `json:"downloadPath"`
	Checksum         string `json:"checksum"`
	MimeType         string `json:"mimeType"`
	Description      string `json:"description,omitempty"`
	Remove           bool   `json:"remove,omitempty"`
	ReplaceVariables bool   `json:"replaceVariables,omitempty"`
}
