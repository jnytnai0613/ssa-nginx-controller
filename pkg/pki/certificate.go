package pki

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"

	//"log"
	"math/big"
	"time"

	ssanginxv1 "github.com/jnytnai0613/ssa-nginx-controller/api/v1"
)

var (
	caTempl      = &x509.Certificate{}
	privateCaKey *rsa.PrivateKey
)

func CreateCaCrt() ([]byte, []byte, error) {
	var err error

	privateCaKey, err = rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}
	publicCaKey := privateCaKey.Public()

	//[RFC5280]
	subjectCa := pkix.Name{
		CommonName:         "ca",
		OrganizationalUnit: []string{"Example Org Unit"},
		Organization:       []string{"Example Org"},
		Country:            []string{"JP"},
	}

	caTempl = &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               subjectCa,
		NotAfter:              time.Date(2031, 12, 31, 0, 0, 0, 0, time.UTC),
		NotBefore:             time.Now(),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}

	//Self Sign CA Certificate
	caCertificate, err := x509.CreateCertificate(rand.Reader, caTempl, caTempl, publicCaKey, privateCaKey)
	if err != nil {
		return nil, nil, err
	}

	//Convert to ASN.1 PEM encoded form
	caCrt := new(bytes.Buffer)
	if err = pem.Encode(caCrt, &pem.Block{Type: "CERTIFICATE", Bytes: caCertificate}); err != nil {
		return nil, nil, err
	}

	derCaPrivateKey := x509.MarshalPKCS1PrivateKey(privateCaKey)

	caKey := new(bytes.Buffer)
	if err = pem.Encode(caKey, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: derCaPrivateKey}); err != nil {
		return nil, nil, err
	}

	return caCrt.Bytes(), caKey.Bytes(), nil
}

func CreateSvrCrt(ssanginx ssanginxv1.SSANginx) ([]byte, []byte, error) {
	privateSvrKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}
	publicSvrKey := privateSvrKey.Public()

	subjectSvr := pkix.Name{
		CommonName:         "server",
		OrganizationalUnit: []string{"Example Org Unit"},
		Organization:       []string{"Example Org"},
		Country:            []string{"JP"},
	}

	svrTempl := &x509.Certificate{
		SerialNumber: big.NewInt(123),
		Subject:      subjectSvr,
		NotAfter:     time.Date(2031, 12, 31, 0, 0, 0, 0, time.UTC),
		NotBefore:    time.Now(),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{*ssanginx.Spec.IngressSpec.Rules[0].Host},
	}

	//SSL Certificate
	derSslCertificate, err := x509.CreateCertificate(rand.Reader, svrTempl, caTempl, publicSvrKey, privateCaKey)
	if err != nil {
		return nil, nil, err
	}

	//Convert to ASN.1 PEM encoded form
	sslCrt := new(bytes.Buffer)
	if err = pem.Encode(sslCrt, &pem.Block{Type: "CERTIFICATE", Bytes: derSslCertificate}); err != nil {
		return nil, nil, err
	}

	derPrivateSslKey := x509.MarshalPKCS1PrivateKey(privateSvrKey)

	sslKey := new(bytes.Buffer)
	if err = pem.Encode(sslKey, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: derPrivateSslKey}); err != nil {
		return nil, nil, err
	}

	return sslCrt.Bytes(), sslKey.Bytes(), nil
}

func CreateClientCrt() ([]byte, []byte, error) {
	privateClientKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}
	publicClientKey := privateClientKey.Public()

	//Client Certificate
	subjectClient := pkix.Name{
		CommonName:         "client",
		OrganizationalUnit: []string{"Example Org Unit"},
		Organization:       []string{"Example Org"},
		Country:            []string{"JP"},
	}

	cliTempl := &x509.Certificate{
		SerialNumber: big.NewInt(456),
		Subject:      subjectClient,
		NotAfter:     time.Date(2031, 12, 31, 0, 0, 0, 0, time.UTC),
		NotBefore:    time.Now(),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	derClientCertificate, err := x509.CreateCertificate(rand.Reader, cliTempl, caTempl, publicClientKey, privateCaKey)
	if err != nil {
		return nil, nil, err
	}

	//Convert to ASN.1 PEM encoded form
	cliCrt := new(bytes.Buffer)
	if err = pem.Encode(cliCrt, &pem.Block{Type: "CERTIFICATE", Bytes: derClientCertificate}); err != nil {
		return nil, nil, err
	}

	derClientPrivateKey := x509.MarshalPKCS1PrivateKey(privateClientKey)

	cliKey := new(bytes.Buffer)
	if err = pem.Encode(cliKey, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: derClientPrivateKey}); err != nil {
		return nil, nil, err
	}

	return cliCrt.Bytes(), cliKey.Bytes(), nil
}
