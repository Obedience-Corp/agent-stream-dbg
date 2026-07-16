package mapping

import "fmt"

// SendSpec declares how a dialect turns a typed message into a request
// that initiates a turn. When a dialect declares no send: block, Send is
// nil and the dialect is watch-only — RenderSend errors clearly rather than
// guessing at a request shape.
type SendSpec struct {
	Method string
	URL    string
	Body   string // empty = no body (GET-style delivery)
}

// sendYAML is the raw YAML shape of a send: block, strictly decoded.
type sendYAML struct {
	Request struct {
		Method string `yaml:"method,omitempty"`
		URL    string `yaml:"url"`
		Body   string `yaml:"body,omitempty"`
	} `yaml:"request"`
}

func (s sendYAML) compile() SendSpec {
	method := s.Request.Method
	if method == "" {
		method = "GET"
	}
	return SendSpec{
		Method: method,
		URL:    s.Request.URL,
		Body:   s.Request.Body,
	}
}

// RenderSend renders a dialect's send: block into a transport-ready
// request: method, URL, and body (nil if the dialect declares no body —
// GET-style delivery). Fully transport-ignorant: the caller decides how to
// execute the rendered request (raw HTTP today; a Transport.Send(ctx,
// payload []byte) once phase 004's transport layer lands). Dialects
// without a send: block are watch-only — RenderSend errors clearly instead
// of guessing at a request shape.
func RenderSend(spec *SendSpec, vars InterpolationVars) (method, url string, body []byte, err error) {
	if spec == nil {
		return "", "", nil, fmt.Errorf("send: dialect declares no send: block (watch-only; cannot send a message)")
	}

	renderedURL, err := InterpolateURL(spec.URL, vars)
	if err != nil {
		return "", "", nil, fmt.Errorf("send: render url: %w", err)
	}

	var renderedBody []byte
	if spec.Body != "" {
		b, err := InterpolateJSON(spec.Body, vars)
		if err != nil {
			return "", "", nil, fmt.Errorf("send: render body: %w", err)
		}
		renderedBody = []byte(b)
	}

	return spec.Method, renderedURL, renderedBody, nil
}
