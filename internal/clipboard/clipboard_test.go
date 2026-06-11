package clipboard

import "testing"

func TestDetectContentType(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{
			name: "png magic number",
			data: []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A},
			want: "image/png",
		},
		{
			name: "jpeg magic number",
			data: []byte{0xFF, 0xD8, 0xFF, 0xE0},
			want: "image/jpeg",
		},
		{
			name: "gif magic number",
			data: []byte("GIF89a"),
			want: "image/gif",
		},
		{
			name: "unrecognized data defaults to png",
			data: []byte("not an image at all"),
			want: "image/png",
		},
		{
			name: "empty data defaults to png",
			data: nil,
			want: "image/png",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := detectContentType(tt.data); got != tt.want {
				t.Errorf("detectContentType() = %q, want %q", got, tt.want)
			}
		})
	}
}
