package keystone

import (
	"encoding/base64"

	"math/rand"
	"time"
)

// GenerateFernetKey -
func GenerateFernetKey() string {
	rand.Seed(time.Now().UnixNano())
	data := make([]byte, 32)
	for i := 0; i < 32; i++ {
		data[i] = byte(rand.Intn(10))
	}
	return base64.StdEncoding.EncodeToString(data)
}
