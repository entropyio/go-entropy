package adapters

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/entropyio/go-entropy/server/p2p/simulations/pipes"
	"sync"
	"testing"
)

func TestTCPPipe(t *testing.T) {
	c1, c2, err := pipes.TCPPipe()
	if err != nil {
		t.Fatal(err)
	}

	msgs := 50
	size := 1024
	for i := 0; i < msgs; i++ {
		msg := make([]byte, size)
		binary.PutUvarint(msg, uint64(i))
		if _, err := c1.Write(msg); err != nil {
			t.Fatal(err)
		}
	}

	for i := 0; i < msgs; i++ {
		msg := make([]byte, size)
		binary.PutUvarint(msg, uint64(i))
		out := make([]byte, size)
		if _, err := c2.Read(out); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(msg, out) {
			t.Fatalf("expected %#v, got %#v", msg, out)
		}
	}
}

func TestTCPPipeBidirections(t *testing.T) {
	c1, c2, err := pipes.TCPPipe()
	if err != nil {
		t.Fatal(err)
	}

	msgs := 50
	size := 7
	for i := 0; i < msgs; i++ {
		msg := []byte(fmt.Sprintf("ping %02d", i))
		if _, err := c1.Write(msg); err != nil {
			t.Fatal(err)
		}
	}

	for i := 0; i < msgs; i++ {
		expected := []byte(fmt.Sprintf("ping %02d", i))
		out := make([]byte, size)
		if _, err := c2.Read(out); err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(expected, out) {
			t.Fatalf("expected %#v, got %#v", out, expected)
		} else {
			msg := []byte(fmt.Sprintf("pong %02d", i))
			if _, err := c2.Write(msg); err != nil {
				t.Fatal(err)
			}
		}
	}

	for i := 0; i < msgs; i++ {
		expected := []byte(fmt.Sprintf("pong %02d", i))
		out := make([]byte, size)
		if _, err := c1.Read(out); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(expected, out) {
			t.Fatalf("expected %#v, got %#v", out, expected)
		}
	}
}

func TestNetPipe(t *testing.T) {
	c1, c2, err := pipes.NetPipe()
	if err != nil {
		t.Fatal(err)
	}

	msgs := 50
	size := 1024
	var wg sync.WaitGroup
	defer wg.Wait()

	// netPipe is blocking, so writes are emitted asynchronously
	wg.Add(1)
	go func() {
		defer wg.Done()

		for i := 0; i < msgs; i++ {
			msg := make([]byte, size)
			binary.PutUvarint(msg, uint64(i))
			if _, err := c1.Write(msg); err != nil {
				t.Error(err)
			}
		}
	}()

	for i := 0; i < msgs; i++ {
		msg := make([]byte, size)
		binary.PutUvarint(msg, uint64(i))
		out := make([]byte, size)
		if _, err := c2.Read(out); err != nil {
			t.Error(err)
		}
		if !bytes.Equal(msg, out) {
			t.Errorf("expected %#v, got %#v", msg, out)
		}
	}
}

func TestNetPipeBidirections(t *testing.T) {
	c1, c2, err := pipes.NetPipe()
	if err != nil {
		t.Fatal(err)
	}

	msgs := 1000
	size := 8
	pingTemplate := "ping %03d"
	pongTemplate := "pong %03d"
	var wg sync.WaitGroup
	defer wg.Wait()

	// netPipe is blocking, so writes are emitted asynchronously
	wg.Add(1)
	go func() {
		defer wg.Done()

		for i := 0; i < msgs; i++ {
			msg := []byte(fmt.Sprintf(pingTemplate, i))
			if _, err := c1.Write(msg); err != nil {
				t.Error(err)
			}
		}
	}()

	// netPipe is blocking, so reads for pong are emitted asynchronously
	wg.Add(1)
	go func() {
		defer wg.Done()

		for i := 0; i < msgs; i++ {
			expected := []byte(fmt.Sprintf(pongTemplate, i))
			out := make([]byte, size)
			if _, err := c1.Read(out); err != nil {
				t.Error(err)
			}
			if !bytes.Equal(expected, out) {
				t.Errorf("expected %#v, got %#v", expected, out)
			}
		}
	}()

	// expect to read pings, and respond with pongs to the alternate connection
	for i := 0; i < msgs; i++ {
		expected := []byte(fmt.Sprintf(pingTemplate, i))

		out := make([]byte, size)
		_, err := c2.Read(out)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(expected, out) {
			t.Errorf("expected %#v, got %#v", expected, out)
		} else {
			msg := []byte(fmt.Sprintf(pongTemplate, i))
			if _, err := c2.Write(msg); err != nil {
				t.Fatal(err)
			}
		}
	}
}
