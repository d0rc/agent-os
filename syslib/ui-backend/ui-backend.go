package ui_backend

import (
	"time"
)

// ClientRequest represents a generic request from the client, encapsulating all specific requests.
type ClientRequest struct {
	Token                      string                      `json:"token,omitempty"`
	LoginRequest               *LoginRequest               `json:"login_request,omitempty"`
	UploadDocumentRequest      *UploadDocumentRequest      `json:"upload_document_request,omitempty"`
	CheckDocumentStatusRequest *CheckDocumentStatusRequest `json:"check_document_status_request,omitempty"`
	ChangeDocumentTagsRequest  *ChangeDocumentTagsRequest  `json:"change_document_tags_request,omitempty"`
	ListDocumentsRequest       *ListDocumentsRequest       `json:"list_documents_request,omitempty"`
	SearchDocumentsRequest     *SearchDocumentsRequest     `json:"search_documents_request,omitempty"`
}

// ServerResponse represents a generic response from the server, encapsulating all specific responses.
type ServerResponse struct {
	LoginResponse               *LoginResponse               `json:"login_response,omitempty"`
	UploadDocumentResponse      *UploadDocumentResponse      `json:"upload_document_response,omitempty"`
	CheckDocumentStatusResponse *CheckDocumentStatusResponse `json:"check_document_status_response,omitempty"`
	ChangeDocumentTagsResponse  *ChangeDocumentTagsResponse  `json:"change_document_tags_response,omitempty"`
	ListDocumentsResponse       *ListDocumentsResponse       `json:"list_documents_response,omitempty"`
	SearchDocumentsResponse     *SearchDocumentsResponse     `json:"search_documents_response,omitempty"`
}

// LoginRequest represents a request for user login.
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse represents a response for a login request.
type LoginResponse struct {
	Success bool   `json:"success"`
	Token   string `json:"token,omitempty"`
	Message string `json:"message"`
}

// UploadDocumentRequest represents a request to upload a document.
type UploadDocumentRequest struct {
	Document Document `json:"document"`
	Tags     []string `json:"tags"`
}

// UploadDocumentResponse represents a response for a document upload request.
type UploadDocumentResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	DocID   string `json:"doc_id"`
}

// Document represents a document with an ID, name, tags, and other metadata.
type Document struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Tags        []string  `json:"tags"`
	UploadTime  time.Time `json:"upload_time"`
	Status      string    `json:"status"` // e.g., "processed", "pending"
	Progress    float64   `json:"progress"`
	Comment     string    `json:"comment"`
	ContentType string    `json:"content_type"` // e.g., "text/plain", "application/pdf"
}

// CheckDocumentStatusRequest represents a request to check the status of a document.
type CheckDocumentStatusRequest struct {
	DocID string `json:"doc_id"`
}

// CheckDocumentStatusResponse represents a response for a document status check request.
type CheckDocumentStatusResponse struct {
	Status   string  `json:"status"`
	Progress float64 `json:"progress"`
	Comment  string  `json:"comment"`
}

// ChangeDocumentTagsRequest represents a request to change the tags of a document.
type ChangeDocumentTagsRequest struct {
	DocID string   `json:"doc_id"`
	Tags  []string `json:"tags"`
}

// ChangeDocumentTagsResponse represents a response for a change document tags request.
type ChangeDocumentTagsResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// ListDocumentsRequest represents a request to list documents, optionally filtered by tags or name.
type ListDocumentsRequest struct {
	FilterBy string   `json:"filter_by"` // "tags" or "name"
	Value    []string `json:"value"`     // Tags or name
}

// ListDocumentsResponse represents a response for a list documents request.
type ListDocumentsResponse struct {
	Documents []Document `json:"documents"`
}

// SearchDocumentsRequest represents a request to search documents by a text string.
type SearchDocumentsRequest struct {
	SearchString string `json:"search_string"`
}

// SearchDocumentsResponse represents a response for a search documents request.
type SearchDocumentsResponse struct {
	Documents []Document `json:"documents"`
}
