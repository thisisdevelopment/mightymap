package storage

import (
	"testing"
)

func TestMsgpackDecodeValue_Error(t *testing.T) {
	_, err := msgpackDecodeValue[int]([]byte{0xff})
	if err == nil {
		t.Error("Expected error when decoding invalid msgpack data, got nil")
	}
}
