package graph

import "testing"

func TestMessageImages(t *testing.T) {
	tests := []struct {
		name string
		msg  Message
		want []ImageRef
	}{
		{
			name: "inline img tag with alt",
			msg: Message{
				Body: MessageBody{
					ContentType: "html",
					Content:     `<div>hi <img src="https://example/x.png" alt="cat.png" width="200"></div>`,
				},
			},
			want: []ImageRef{{Name: "cat.png", URL: "https://example/x.png"}},
		},
		{
			name: "image attachment",
			msg: Message{
				Attachments: []Attachment{
					{
						ContentType: "image/png",
						ContentURL:  "https://example/y.png",
						Name:        "dog.png",
					},
				},
			},
			want: []ImageRef{{Name: "dog.png", URL: "https://example/y.png"}},
		},
		{
			name: "no images",
			msg:  Message{Body: MessageBody{ContentType: "text", Content: "just text"}},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.msg.Images()
			if len(got) != len(tt.want) {
				t.Fatalf("Images() = %v, want %v", got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("Images()[%d] = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
			if tt.msg.HasImages() != (len(tt.want) > 0) {
				t.Errorf("HasImages() = %v, want %v", tt.msg.HasImages(), len(tt.want) > 0)
			}
		})
	}
}
