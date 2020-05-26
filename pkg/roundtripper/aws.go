package roundtripper

import (
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go/aws/credentials"
	v4 "github.com/aws/aws-sdk-go/aws/signer/v4"
)

const (
	service = "es"
)

type AWSSigningTransport struct {
	DefaultTransport *http.Transport
	Credentials      *credentials.Credentials
	Region           string
}

// RoundTrip implementation
func (a AWSSigningTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	signer := v4.NewSigner(a.Credentials)

	// body is nil as we never send data to Elastic, just get
	if _, err := signer.Sign(req, nil, service, a.Region, time.Now()); err != nil {
		return nil, err
	}

	return a.DefaultTransport.RoundTrip(req)
}
