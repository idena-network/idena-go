package crypto

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"testing"
	"time"
)

func Test_Speeed(t *testing.T) {

	var data [][]byte

	for i := 0; i < 100000; i++ {
		item := make([]byte, 100)
		rand.Read(item)
		data = append(data, item)
	}

	timer := time.Now()

	for _, item := range data {
		Hash(item)
	}

	fmt.Println(time.Since(timer).String())
	timer = time.Now()

	sha2 := sha256.New()
	for _, item := range data {
		sha2.Reset()
		sha2.Write(item)
		var h [32]byte
		sha2.Sum(h[:0])
	}

	fmt.Println(time.Since(timer).String())
}
