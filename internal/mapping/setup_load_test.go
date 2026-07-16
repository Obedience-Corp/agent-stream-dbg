package mapping

import "testing"

func TestLoad_SetupBlockParsesAndStrict(t *testing.T) {
	engine, err := Load([]byte(`
version: 1
name: test
discriminator: event
setup:
  request:
    method: POST
    url: "{base_url}/api/v3/debug/session"
    body: '{"session_id": "{session_id}", "agents": {agents}, "reuse_existing": true}'
  response:
    require: {path: success, equals: true}
    session_id: session_id
rules:
  - match: {event: x}
    kind: content
`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if engine.Setup == nil {
		t.Fatal("expected Setup to be parsed")
	}
	if engine.Setup.Request.Method != "POST" || engine.Setup.Response.SessionIDPath != "session_id" {
		t.Errorf("setup not parsed correctly: %+v", engine.Setup)
	}

	_, err = Load([]byte(`
version: 1
name: test
discriminator: event
setup:
  request:
    url: "{base_url}/setup"
  bogus: field
rules:
  - match: {event: x}
    kind: content
`))
	if err == nil {
		t.Fatal("expected strict decode error for unknown setup key")
	}
}
