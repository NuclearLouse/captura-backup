package request

import (
	"bytes"
	"net/http"
	"net/url"
	"time"
)

type MethodApi struct {
	Scheme string
	Host   string
	Path   string
	Params map[string]string
}

//Default MethodApi.Scheme = "https"
func MethodApiURL(m *MethodApi) string {
	val := url.Values{}
	for key, value := range m.Params {
		val.Set(key, value)
	}
	if m.Scheme == "" {
		m.Scheme = "https"
	}
	u := url.URL{
		Scheme:   m.Scheme,
		Host:     m.Host,
		Path:     m.Path,
		RawQuery: val.Encode(),
	}
	return u.String()
}

type Params struct {
	Method  string
	URL     string
	Timeout int64
	Body    []byte
	Header  map[string]string
}

// Default Params.Method = "GET"
func NewRequest(p *Params) (*http.Response, error) {
	if p.Method == "" {
		p.Method = "GET"
	}

	req, err := http.NewRequest(p.Method, p.URL, bytes.NewBuffer(p.Body))
	if err != nil {
		return nil, err
	}

	if p.Header != nil {
		for key, value := range p.Header {
			req.Header.Set(key, value)
		}
	}

	client := http.Client{
		Timeout: time.Duration(p.Timeout) * time.Second,
	}

	return client.Do(req)
}
