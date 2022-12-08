package mydburl_test

import (
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cli/safeexec"
	"github.com/go-sql-driver/mysql"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/ory/dockertest/v3"
	"github.com/pepabo/mydburl"
	"github.com/xo/dburl"
)

const (
	mysqlRootPassword = "rootpass"
	mysqlUser         = "myuser"
	mysqlPassword     = "mypass"
	mysqlDatabase     = "testdb"
)

func TestParse(t *testing.T) {
	tests := []struct {
		dsn     string
		want    *mydburl.URL
		wantErr bool
	}{
		{"mysql://username:password@localhost:3306/testdb?parseTime=true", newURL(t, "mysql://username:password@localhost:3306/testdb?parseTime=true", "", "", ""), false},
		{"mysql://username:password@localhost:3306/testdb?tls=true", newURL(t, "mysql://username:password@localhost:3306/testdb?tls=true", "", "", ""), false},
		{"mysql://username:password@localhost:3306/testdb?sslCa=testdata/certs/root-ca.pem", newURL(t, "mysql://username:password@localhost:3306/testdb?sslCa=testdata/certs/root-ca.pem", "testdata/certs/root-ca.pem", "", ""), false},
		{"my://username:password@localhost:3306/testdb?sslCa=testdata/certs/root-ca.pem", newURL(t, "my://username:password@localhost:3306/testdb?sslCa=testdata/certs/root-ca.pem", "testdata/certs/root-ca.pem", "", ""), false},
		{"mysql://username:password@localhost:3306/testdb?sslCa=testdata/certs/root-ca.pem&sslCert=testdata/certs/client-cert.pem&sslKey=testdata/certs/client-key.pem", newURL(t, "mysql://username:password@localhost:3306/testdb?sslCa=testdata/certs/root-ca.pem&sslCert=testdata/certs/client-cert.pem&sslKey=testdata/certs/client-key.pem", "testdata/certs/root-ca.pem", "testdata/certs/client-cert.pem", "testdata/certs/client-key.pem"), false},
		{"mysql://username:password@localhost:3306/testdb?sslCa=path/to/notexist.pem", nil, true},
		{"mysql://username:password@localhost:3306/testdb?tls=true&sslCa=testdata/certs/root-ca.pem", nil, true},
		{"mysql://username:password@localhost:3306/testdb?sslCa=testdata/certs/root-ca.pem&sslCert=testdata/certs/client-cert.pem", nil, true},
		{"pg://username:password@localhost:3306/testdb", newURL(t, "pg://username:password@localhost:3306/testdb", "", "", ""), false},
		{"pg://username:password@localhost:3306/testdb?sslCa=testdata/certs/root-ca.pem", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.dsn, func(t *testing.T) {
			got, err := mydburl.Parse(tt.dsn)
			if err != nil {
				if tt.wantErr {
					return
				}
				t.Error(err)
			}
			if tt.wantErr {
				t.Error("want err")
			}
			dopts := []cmp.Option{
				cmpopts.IgnoreUnexported(dburl.URL{}, url.Userinfo{}),
			}
			if diff := cmp.Diff(got, tt.want, dopts...); diff != "" {
				t.Errorf("%s", diff)
			}
		})
	}
}

func TestOpen(t *testing.T) {
	port, ca, cert, key := createMySQLContainer(t)
	tests := []struct {
		dsn     string
		wantErr bool
	}{
		{fmt.Sprintf("mysql://%s:%s@localhost:%s/%s?parseTime=true", mysqlUser, mysqlPassword, port, mysqlDatabase), true},
		{fmt.Sprintf("mysql://%s:%s@localhost:%s/%s?parseTime=true&sslCa=%s", mysqlUser, mysqlPassword, port, mysqlDatabase, ca), false},
		{fmt.Sprintf("mysql://%s:%s@localhost:%s/%s?parseTime=true&sslCa=%s&sslCert=%s&sslKey=%s", mysqlUser, mysqlPassword, port, mysqlDatabase, ca, cert, key), false},
	}
	for _, tt := range tests {
		t.Run(tt.dsn, func(t *testing.T) {
			db, err := mydburl.Open(tt.dsn)
			if err != nil {
				t.Fatal(err)
			}
			if err := db.Ping(); err != nil {
				if tt.wantErr {
					return
				}
				t.Error(err)
			}
			if tt.wantErr {
				t.Error("want err")
			}
		})
	}
}

func TestRegisterTlsConfig(t *testing.T) {
	port, ca, cert, key := createMySQLContainer(t)
	tests := []struct {
		dsn     string
		wantErr bool
	}{
		{fmt.Sprintf("mysql://%s:%s@localhost:%s/%s?parseTime=true", mysqlUser, mysqlPassword, port, mysqlDatabase), true},
		{fmt.Sprintf("mysql://%s:%s@localhost:%s/%s?parseTime=true&sslCa=%s", mysqlUser, mysqlPassword, port, mysqlDatabase, ca), false},
		{fmt.Sprintf("mysql://%s:%s@localhost:%s/%s?parseTime=true&sslCa=%s&sslCert=%s&sslKey=%s", mysqlUser, mysqlPassword, port, mysqlDatabase, ca, cert, key), false},
	}
	for _, tt := range tests {
		t.Run(tt.dsn, func(t *testing.T) {
			u, err := mydburl.Parse(tt.dsn)
			if err != nil {
				t.Fatal(err)
			}
			if err := u.RegisterTlsConfig("test"); err != nil {
				if tt.wantErr {
					return
				}
				t.Error(err)
			}
			if strings.Contains(u.String(), "sslCa") {
				t.Error("sslCa should be removed")
			}
			if !strings.Contains(u.String(), "tls") {
				t.Error("tls should be added")
			}
			db, err := sql.Open(u.Driver, u.DSN)
			if err != nil {
				t.Fatal(err)
			}
			if err := db.Ping(); err != nil {
				t.Error(err)
			}
		})
	}
}

// return port, ca, cert, key
func createMySQLContainer(t *testing.T) (string, string, string, string) {
	t.Helper()
	pool, err := dockertest.NewPool("")
	if err != nil {
		t.Fatalf("Could not connect to docker: %s", err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	opt := &dockertest.RunOptions{
		Repository: "mysql",
		Tag:        "8",
		Env: []string{
			fmt.Sprintf("MYSQL_ROOT_PASSWORD=%s", mysqlRootPassword),
			fmt.Sprintf("MYSQL_USER=%s", mysqlUser),
			fmt.Sprintf("MYSQL_PASSWORD=%s", mysqlPassword),
			fmt.Sprintf("MYSQL_DATABASE=%s", mysqlDatabase),
		},
		Mounts: []string{
			fmt.Sprintf("%s:/etc/certs", filepath.Join(wd, "testdata", "certs")),
		},
		Cmd: []string{
			"mysqld",
			"--character-set-server=utf8mb4",
			"--collation-server=utf8mb4_unicode_ci",
			"--require_secure_transport=ON",
			"--ssl-ca=/etc/certs/root-ca.pem",
			"--ssl-cert=/etc/certs/server-cert.pem",
			"--ssl-key=/etc/certs/server-key.pem",
		},
	}
	my, err := pool.RunWithOptions(opt)
	if err != nil {
		t.Fatalf("Could not start resource: %s", err)
	}
	t.Cleanup(func() {
		if os.Getenv("DEBUG") != "" {
			c, err := safeexec.LookPath("docker")
			if err != nil {
				t.Error(err)
			}
			cmd := exec.Command(c, "logs", my.Container.ID)
			b, err := cmd.CombinedOutput()
			if err != nil {
				t.Error(err)
			}
			fmt.Println("------------")
			fmt.Println(string(b))
			fmt.Println("------------")
		}

		if err := pool.Purge(my); err != nil {
			t.Fatalf("Could not purge resource: %s", err)
		}
	})

	var port, ca, cert, key string
	tlsKey := "testcontainer"
	if err := pool.Retry(func() error {
		time.Sleep(time.Second * 10)
		var err error
		ca, cert, key, err = registerTlsConfig(tlsKey)
		if err != nil {
			t.Log(err)
			return err
		}
		port = my.GetPort("3306/tcp")
		db, err := sql.Open("mysql", fmt.Sprintf("%s:%s@(localhost:%s)/%s?&parseTime=true&tls=%s", mysqlUser, mysqlPassword, port, mysqlDatabase, tlsKey))
		if err != nil {
			t.Log(err)
			return err
		}
		if err := db.Ping(); err != nil {
			t.Log(err)
			return err
		}
		return nil
	}); err != nil {
		t.Fatalf("Could not connect to database: %s", err)
	}

	return port, ca, cert, key
}

func registerTlsConfig(tlsKey string) (string, string, string, error) {
	ca := "testdata/certs/root-ca.pem"
	cert := "testdata/certs/client-cert.pem"
	key := "testdata/certs/client-key.pem"

	rootCertPool := x509.NewCertPool()
	pem, err := os.ReadFile(ca)
	if err != nil {
		return "", "", "", err
	}
	if ok := rootCertPool.AppendCertsFromPEM(pem); !ok {
		return "", "", "", errors.New("Failed to append PEM.")
	}
	clientCert := make([]tls.Certificate, 0, 1)
	certs, err := tls.LoadX509KeyPair(cert, key)
	if err != nil {
		return "", "", "", err
	}
	clientCert = append(clientCert, certs)
	if err := mysql.RegisterTLSConfig(tlsKey, &tls.Config{
		RootCAs:      rootCertPool,
		Certificates: clientCert,
		MinVersion:   tls.VersionTLS12,
	}); err != nil {
		return "", "", "", err
	}

	return ca, cert, key, nil
}

func newURL(t *testing.T, urlstr, ca, cert, key string) *mydburl.URL {
	v, err := dburl.Parse(urlstr)
	if err != nil {
		t.Fatal(err)
	}
	return &mydburl.URL{
		URL:     *v,
		SslCa:   ca,
		SslCert: cert,
		SslKey:  key,
	}
}
