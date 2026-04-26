package llm

type Response struct {
	Response string  `json:"response"`
	Dollars  float64 `json:"dollars"`
}

type File struct {
	Path     string `json:"path"`
	MIMEType string `json:"mimeType"`
}

type UploadedFile struct {
	URI      string `json:"uri"`
	MIMEType string `json:"mimeType"`
}
