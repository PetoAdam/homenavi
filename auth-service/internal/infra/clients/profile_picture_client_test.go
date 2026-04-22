package clients

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"testing"
)

func TestUploadProfilePicturePreservesContentType(t *testing.T) {
	t.Helper()

	receivedContentType := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/upload" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := r.ParseMultipartForm(2 << 20); err != nil {
			t.Fatalf("ParseMultipartForm() error = %v", err)
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("FormFile() error = %v", err)
		}
		_ = file.Close()
		receivedContentType = header.Header.Get("Content-Type")

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success":     true,
			"primary_url": "/api/profile-pictures/users/test-user?v=123",
		})
	}))
	defer server.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	partHeaders := textproto.MIMEHeader{}
	partHeaders.Set("Content-Disposition", `form-data; name="file"; filename="avatar.png"`)
	partHeaders.Set("Content-Type", "image/png")
	part, err := writer.CreatePart(partHeaders)
	if err != nil {
		t.Fatalf("CreatePart() error = %v", err)
	}
	if _, err := part.Write([]byte("fake-image-payload")); err != nil {
		t.Fatalf("part.Write() error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if err := req.ParseMultipartForm(2 << 20); err != nil {
		t.Fatalf("req.ParseMultipartForm() error = %v", err)
	}
	file, header, err := req.FormFile("file")
	if err != nil {
		t.Fatalf("req.FormFile() error = %v", err)
	}
	defer file.Close()

	client := NewProfilePictureClient(server.URL)
	if _, err := client.UploadProfilePicture("test-user", file, header); err != nil {
		t.Fatalf("UploadProfilePicture() error = %v", err)
	}

	if receivedContentType != "image/png" {
		t.Fatalf("expected forwarded content type image/png, got %q", receivedContentType)
	}
}
