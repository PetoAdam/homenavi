package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"unicode/utf8"
)

const websocketMessageLimit = 4 << 20

type websocket struct {
	connection io.ReadWriteCloser
	reader     *bufio.Reader
	writeMu    sync.Mutex
	stop       func() bool
}

func openWebsocket(ctx context.Context, endpoint string) (*websocket, error) {
	keyBytes := make([]byte, 16)
	if _, err := rand.Read(keyBytes); err != nil {
		return nil, err
	}
	key := base64.StdEncoding.EncodeToString(keyBytes)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Connection", "Upgrade")
	request.Header.Set("Upgrade", "websocket")
	request.Header.Set("Sec-WebSocket-Version", "13")
	request.Header.Set("Sec-WebSocket-Key", key)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusSwitchingProtocols {
		response.Body.Close()
		return nil, fmt.Errorf("Open WebUI websocket returned %s", response.Status)
	}
	acceptHash := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	connectionUpgrade := false
	for _, value := range strings.Split(response.Header.Get("Connection"), ",") {
		if strings.EqualFold(strings.TrimSpace(value), "upgrade") {
			connectionUpgrade = true
		}
	}
	if !connectionUpgrade || !strings.EqualFold(response.Header.Get("Upgrade"), "websocket") || response.Header.Get("Sec-WebSocket-Accept") != base64.StdEncoding.EncodeToString(acceptHash[:]) || response.Header.Get("Sec-WebSocket-Extensions") != "" {
		response.Body.Close()
		return nil, errors.New("invalid Open WebUI websocket handshake")
	}
	connection, ok := response.Body.(io.ReadWriteCloser)
	if !ok {
		response.Body.Close()
		return nil, errors.New("Open WebUI websocket connection is not writable")
	}
	socket := &websocket{connection: connection, reader: bufio.NewReader(connection)}
	socket.stop = context.AfterFunc(ctx, func() { _ = connection.Close() })
	return socket, nil
}

func (socket *websocket) close() {
	socket.stop()
	_ = socket.connection.Close()
}

func (socket *websocket) write(opcode byte, payload []byte) error {
	if len(payload) > websocketMessageLimit || (opcode >= 8 && len(payload) > 125) {
		return errors.New("websocket message exceeds size limit")
	}
	mask := make([]byte, 4)
	if _, err := rand.Read(mask); err != nil {
		return err
	}
	frame := []byte{0x80 | opcode}
	switch {
	case len(payload) < 126:
		frame = append(frame, 0x80|byte(len(payload)))
	case len(payload) <= 65535:
		frame = append(frame, 0x80|126, byte(len(payload)>>8), byte(len(payload)))
	default:
		frame = append(frame, 0x80|127)
		frame = binary.BigEndian.AppendUint64(frame, uint64(len(payload)))
	}
	frame = append(frame, mask...)
	for index, value := range payload {
		frame = append(frame, value^mask[index%4])
	}
	socket.writeMu.Lock()
	defer socket.writeMu.Unlock()
	for len(frame) > 0 {
		written, err := socket.connection.Write(frame)
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
		frame = frame[written:]
	}
	return nil
}

func (socket *websocket) read() (string, error) {
	var message []byte
	fragmented := false
	for {
		header := make([]byte, 2)
		if _, err := io.ReadFull(socket.reader, header); err != nil {
			return "", err
		}
		final := header[0]&0x80 != 0
		opcode := header[0] & 0x0f
		if header[0]&0x70 != 0 || header[1]&0x80 != 0 {
			return "", errors.New("invalid websocket frame flags")
		}
		length := uint64(header[1] & 0x7f)
		if length == 126 {
			var extended [2]byte
			if _, err := io.ReadFull(socket.reader, extended[:]); err != nil {
				return "", err
			}
			length = uint64(binary.BigEndian.Uint16(extended[:]))
		} else if length == 127 {
			var extended [8]byte
			if _, err := io.ReadFull(socket.reader, extended[:]); err != nil {
				return "", err
			}
			length = binary.BigEndian.Uint64(extended[:])
		}
		if length > websocketMessageLimit || uint64(len(message))+length > websocketMessageLimit || (opcode >= 8 && (!final || length > 125)) {
			return "", errors.New("invalid or oversized websocket frame")
		}
		payload := make([]byte, int(length))
		if _, err := io.ReadFull(socket.reader, payload); err != nil {
			return "", err
		}
		switch opcode {
		case 8:
			return "", errors.New("Open WebUI websocket closed")
		case 9:
			if err := socket.write(10, payload); err != nil {
				return "", err
			}
			continue
		case 10:
			continue
		case 1:
			if fragmented {
				return "", errors.New("unexpected websocket text frame")
			}
			fragmented = !final
		case 0:
			if !fragmented {
				return "", errors.New("unexpected websocket continuation frame")
			}
		default:
			return "", errors.New("unsupported websocket frame")
		}
		message = append(message, payload...)
		if final {
			if !utf8.Valid(message) {
				return "", errors.New("invalid websocket text encoding")
			}
			return string(message), nil
		}
	}
}
