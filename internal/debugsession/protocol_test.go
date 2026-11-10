package debugsession

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWireMessagesEncoding(t *testing.T) {
	tests := []struct {
		name string
		msg  any
		want string
	}{
		{
			name: "a response without headers or payload",
			msg:  responseMessage{ID: "r1", Operation: "execute-response", Status: 200},
			want: `{"id":"r1","operation":"execute-response","status":200,"headers":null,"payload":null}`,
		},
		{
			name: "a response with an empty payload",
			msg:  responseMessage{ID: "r1", Operation: "execute-response", Status: 204, Payload: []byte{}},
			want: `{"id":"r1","operation":"execute-response","status":204,"headers":null,"payload":""}`,
		},
		{
			name: "a response with headers and a payload",
			msg: responseMessage{
				ID: "r2", Operation: "execute-response", Status: 201,
				Headers: map[string][]string{"X-App": {"a", "b"}}, Payload: []byte("hi"),
			},
			want: `{"id":"r2","operation":"execute-response","status":201,"headers":{"X-App":["a","b"]},"payload":"aGk="}`,
		},
		{
			name: "a remote request",
			msg: remoteRequestMessage{
				ID: "x1", Operation: "execute-remote",
				Request: remoteCommand{FAASFunction: "provider-a", URL: "http://provider-a/hook", Method: "POST", Payload: []byte("hi")},
				Headers: map[string]string{"Accept": "text/plain"},
				Query:   map[string][]string{"b": {"2"}},
			},
			want: `{"id":"x1","operation":"execute-remote","request":{"faas_function":"provider-a","url":"http://provider-a/hook",` +
				`"method":"POST","payload":"aGk="},"headers":{"Accept":"text/plain"},"query":{"b":["2"]}}`,
		},
		{
			name: "a remote request without payload, headers or query",
			msg:  remoteRequestMessage{ID: "x2", Operation: "execute-remote", Request: remoteCommand{Method: "GET"}},
			want: `{"id":"x2","operation":"execute-remote","request":{"faas_function":"","url":"","method":"GET","payload":null},` +
				`"headers":null,"query":null}`,
		},
		{
			name: "a remote request with an empty payload",
			msg:  remoteRequestMessage{ID: "x3", Operation: "execute-remote", Request: remoteCommand{Payload: []byte{}}},
			want: `{"id":"x3","operation":"execute-remote","request":{"faas_function":"","url":"","method":"","payload":""},` +
				`"headers":null,"query":null}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.msg)

			require.NoError(t, err)
			assert.Equal(t, tt.want, string(got))
		})
	}
}

func TestWireMessagesDecoding(t *testing.T) {
	t.Run("a request with every field", func(t *testing.T) {
		frame := `{"id":"r1","operation":"execute","method":"POST","route":"/hook","query":{"a":["1","2"]},` +
			`"headers":{"X":["y"]},"payload":"aGVsbG8=","trace":{"unknown":true}}`

		var msg requestMessage
		require.NoError(t, json.Unmarshal([]byte(frame), &msg))

		assert.Equal(t, requestMessage{
			ID: "r1", Operation: "execute", Method: "POST", Route: "/hook",
			Query: map[string][]string{"a": {"1", "2"}}, Headers: map[string][]string{"X": {"y"}}, Payload: []byte("hello"),
		}, msg)
	})

	t.Run("an absent or null payload is empty", func(t *testing.T) {
		for _, frame := range []string{
			`{"id":"r1","operation":"execute","method":"GET","route":"/"}`,
			`{"id":"r1","operation":"execute","method":"GET","route":"/","payload":null}`,
		} {
			var msg requestMessage
			require.NoError(t, json.Unmarshal([]byte(frame), &msg))

			assert.Empty(t, msg.Payload, frame)
		}
	})

	t.Run("a response", func(t *testing.T) {
		frame := `{"id":"x1","operation":"execute-remote","status":202,"headers":{"Content-Type":["text/plain"]},"payload":"YWNjZXB0ZWQ=","more":1}`

		var msg responseMessage
		require.NoError(t, json.Unmarshal([]byte(frame), &msg))

		assert.Equal(t, responseMessage{
			ID: "x1", Operation: "execute-remote", Status: 202,
			Headers: map[string][]string{"Content-Type": {"text/plain"}}, Payload: []byte("accepted"),
		}, msg)
	})

	t.Run("a payload that is not base64 is an error", func(t *testing.T) {
		var msg requestMessage

		err := json.Unmarshal([]byte(`{"id":"r1","payload":"not base64!"}`), &msg)

		require.Error(t, err)
	})
}

func TestSessionErrorTexts(t *testing.T) {
	assert.EqualError(t, errTakenOver, "This debug session was taken over by another connection. Stopping.")
	assert.EqualError(t, errConnectionGone, "connection Id not found")
}
