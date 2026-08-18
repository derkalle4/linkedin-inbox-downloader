package browser

import (
	"testing"
)

func TestDecodeConversationsArray(t *testing.T) {
	raw := []byte(`[{"name":"Jane","time":"1m","snippet":"Hi","photo":"","key":"k","href":"https://www.linkedin.com/messaging/thread/abc/"}]`)
	got, err := decodeConversations(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].NameStr() != "Jane" {
		t.Fatalf("unexpected: %+v", got)
	}
	if got[0].HrefStr() != "https://www.linkedin.com/messaging/thread/abc/" {
		t.Fatalf("href=%q", got[0].HrefStr())
	}
}

func TestDecodeConversationsObject(t *testing.T) {
	raw := []byte(`{"0":{"name":"Jane","time":"1m","snippet":"Hi","photo":"","key":"k"}}`)
	got, err := decodeConversations(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].NameStr() != "Jane" {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestDecodeConversationsHrefOnly(t *testing.T) {
	raw := []byte(`[{"name":"Bob","time":"","snippet":"","photo":"","key":"","href":"/messaging/thread/xyz/"}]`)
	got, err := decodeConversations(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].HrefStr() != "/messaging/thread/xyz/" {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestDecodeThreadSoftFields(t *testing.T) {
	raw := []byte(`{"name":"Ada","headline":{"x":1},"items":[{"type":"msg","html":"hi","images":["a",{"b":1}],"self":true}]}`)
	got, err := decodeThread(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Ada" || got.Headline != "" {
		t.Fatalf("unexpected header: %+v", got)
	}
	if len(got.Items) != 1 || got.Items[0].HTML != "hi" || len(got.Items[0].Images) != 1 {
		t.Fatalf("unexpected items: %+v", got.Items)
	}
}

func TestMessageImagesOrder(t *testing.T) {
	data, err := decodeThread([]byte(`{"items":[
		{"type":"day","heading":"Today"},
		{"type":"msg","images":["one",""]},
		{"type":"msg","images":["two","three"]}
	]}`))
	if err != nil {
		t.Fatal(err)
	}
	got := messageImages(data)
	if len(got) != 3 || got[0] != "one" || got[1] != "two" || got[2] != "three" {
		t.Fatalf("got %#v", got)
	}
}
