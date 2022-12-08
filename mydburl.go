package mydburl

import (
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"

	"github.com/go-sql-driver/mysql"
	"github.com/xo/dburl"
)

const (
	DefaultTlsKey = "mydburl"
	tlsKey        = "tls"
	sslCaKey      = "sslCa"
	sslCertKey    = "sslCert"
	sslKeyKey     = "sslKey"
)

type URL struct {
	dburl.URL

	SslCa   string
	SslCert string
	SslKey  string
}

func Parse(urlstr string) (*URL, error) {
	var (
		ca, cert, key string
	)
	v, err := dburl.Parse(urlstr)
	if err != nil {
		return nil, err
	}
	u := &URL{URL: *v}
	if u.Query().Has(sslCaKey) {
		ca = u.Query().Get(sslCaKey)
		if _, err := os.Stat(ca); err != nil {
			return nil, err
		}
	}
	if u.Query().Has(sslCertKey) {
		cert = u.Query().Get(sslCertKey)
		if _, err := os.Stat(cert); err != nil {
			return nil, err
		}
	}
	if u.Query().Has(sslKeyKey) {
		key = u.Query().Get(sslKeyKey)
		if _, err := os.Stat(key); err != nil {
			return nil, err
		}
	}
	if ca == "" && cert == "" && key == "" {
		return u, nil
	}
	if u.Driver != "mysql" {
		return nil, fmt.Errorf("mydburl support only mysql: %s", u.Driver)
	}
	if u.Query().Has(tlsKey) {
		return nil, errors.New("tls cannot be used with sslCa, sslCert, or sslKey")
	}
	if (cert == "") != (key == "") {
		return nil, errors.New("sslCert and sslKey should both set or both unset")
	}

	u.SslCa = ca
	u.SslCert = cert
	u.SslKey = key

	return u, nil
}

func Open(urlstr string) (*sql.DB, error) {
	u, err := Parse(urlstr)
	if err != nil {
		return nil, err
	}
	if u.SslCa == "" {
		return dburl.Open(urlstr)
	}
	if err := u.RegisterTlsConfig(DefaultTlsKey); err != nil {
		return nil, err
	}
	return dburl.Open(u.String())
}

func (u *URL) RegisterTlsConfig(k string) error {
	rootCertPool := x509.NewCertPool()
	pem, err := os.ReadFile(u.SslCa)
	if err != nil {
		return err
	}
	if ok := rootCertPool.AppendCertsFromPEM(pem); !ok {
		return errors.New("failed to append cert from PEM")
	}
	tc := &tls.Config{
		RootCAs:    rootCertPool,
		MinVersion: tls.VersionTLS12,
	}
	if u.SslCert != "" && u.SslKey != "" {
		clientCert := make([]tls.Certificate, 0, 1)
		certs, err := tls.LoadX509KeyPair(u.SslCert, u.SslKey)
		if err != nil {
			return err
		}
		clientCert = append(clientCert, certs)
		tc.Certificates = clientCert
	}
	if err := mysql.RegisterTLSConfig(k, tc); err != nil {
		return err
	}
	uu, err := url.Parse(u.String())
	if err != nil {
		return err
	}
	q := uu.Query()
	q.Add(tlsKey, DefaultTlsKey)
	q.Del(sslCaKey)
	q.Del(sslCertKey)
	q.Del(sslKeyKey)
	uu.RawQuery = q.Encode()
	uuu, err := dburl.Parse(uu.String())
	if err != nil {
		return err
	}
	u.URL = *uuu
	return nil
}
