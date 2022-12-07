package mydburl_test

import (
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/cli/safeexec"
	"github.com/go-sql-driver/mysql"
	"github.com/ory/dockertest/v3"
	"github.com/pepabo/mydburl"
)

const (
	mysqlRootPassword = "rootpass"
	mysqlUser         = "myuser"
	mysqlPassword     = "mypass"
	mysqlDatabase     = "testdb"
)

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
			fmt.Sprintf("%s:/etc/mysql/conf.d", filepath.Join(wd, "testdata", "conf.d")),
			fmt.Sprintf("%s:/docker-entrypoint-initdb.d", filepath.Join(wd, "testdata", "docker-entrypoint-initdb.d")),
			fmt.Sprintf("%s:/etc/certs", filepath.Join(wd, "testdata", "certs")),
		},
		Cmd: []string{
			"mysqld",
			"--character-set-server=utf8mb4",
			"--collation-server=utf8mb4_unicode_ci",
			"--require_secure_transport=ON",
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

func TestOpen(t *testing.T) {
	port, ca, cert, key := createMySQLContainer(t)
	dsn := fmt.Sprintf("mysql://%s:%s@localhost:%s/%s?parseTime=true&sslCa=%s&sslCert=%s&sslKey=%s", mysqlUser, mysqlPassword, port, mysqlDatabase, ca, cert, key)
	db, err := mydburl.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		t.Error(err)
	}
}
