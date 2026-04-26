package llm

type Interface interface {
	UploadFiles(files []File) ([]UploadedFile, error)
	GenerateText(prompt string, useFastModel bool, uploadedFiles []UploadedFile) (*Response, error)
}
