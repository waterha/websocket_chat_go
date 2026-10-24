package local

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

const (
	appDBUser     = "websocket_chat_go"
	appDBName     = "websocket_chat_go"
	pgSuperUser   = "postgres"
	initTimeout   = 2 * time.Minute
	listenTimeout = 60 * time.Second
)

type postgresService struct {
	bin           string
	data          string
	logs          string
	port          int
	superPassword string
	appPassword   string

	// startMu serialises the lifecycle flag against a Stop that may arrive
	// while the cluster is still starting.
	startMu sync.Mutex
	started bool
}

// loadOrCreatePassword returns the secret stored in path, generating one on
// first use. The file keeps the launcher self-sufficient across restarts.
func loadOrCreatePassword(path string) (string, error) {
	if data, err := os.ReadFile(path); err == nil {
		if secret := string(data); secret != "" {
			return secret, nil
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	secret := hex.EncodeToString(buffer)
	if err := os.WriteFile(path, []byte(secret), 0600); err != nil {
		return "", err
	}
	return secret, nil
}

func tcpOpen(port int) bool {
	connection, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 300*time.Millisecond)
	if err != nil {
		return false
	}
	_ = connection.Close()
	return true
}

// runLogged executes a service command with output appended to a log file.
// pg_ctl start leaves PostgreSQL running as a child that inherits handles, so
// pipes would never see EOF; files keep the launcher from blocking.
func (service *postgresService) runLogged(ctx context.Context, logName, name string, arguments ...string) error {
	logPath := filepath.Join(service.logs, logName)
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()
	command := exec.CommandContext(ctx, filepath.Join(service.bin, exeName(name)), arguments...)
	command.Stdout, command.Stderr = file, file
	hideWindow(command)
	if err := command.Run(); err != nil {
		return fmt.Errorf("%s 失败（%v），详见 %s", name, err, logPath)
	}
	return nil
}

func (service *postgresService) pgctl(ctx context.Context, arguments ...string) error {
	// pg_ctl opens its own -l log exclusively, so its own output goes to a
	// separate file to avoid sharing one file with the server log.
	return service.runLogged(ctx, "pg_ctl.log", "pg_ctl", arguments...)
}

// psqlOutput runs a statement as the superuser and returns its output.
func (service *postgresService) psqlOutput(ctx context.Context, statement string) (string, error) {
	command := exec.CommandContext(ctx, filepath.Join(service.bin, exeName("psql")),
		"-h", "127.0.0.1", "-p", strconv.Itoa(service.port), "-U", pgSuperUser, "-d", "postgres",
		"-v", "ON_ERROR_STOP=1", "-q", "-tA", "-c", statement)
	command.Env = append(os.Environ(), "PGPASSWORD="+service.superPassword)
	hideWindow(command)
	var output bytes.Buffer
	command.Stdout, command.Stderr = &output, &output
	if err := command.Run(); err != nil {
		return "", fmt.Errorf("psql: %v\n%s", err, trimOutput(output.Bytes()))
	}
	return output.String(), nil
}

// ensure brings the private cluster up, or reuses whichever PostgreSQL already
// listens on the port, and then aligns the application role and database.
func (service *postgresService) ensure(ctx context.Context, progress func(string)) error {
	if tcpOpen(service.port) {
		progress(fmt.Sprintf("检测到 PostgreSQL 已在运行（端口 %d）", service.port))
		if err := service.ensureDatabase(ctx, progress); err != nil {
			return fmt.Errorf("%w（若端口 %d 被使用其他密码的 PostgreSQL 占用，请先关闭它再重试）", err, service.port)
		}
		return nil
	}
	if err := service.start(ctx, progress); err != nil {
		return err
	}
	return service.ensureDatabase(ctx, progress)
}

func (service *postgresService) start(ctx context.Context, progress func(string)) error {
	if !fileExists(filepath.Join(service.data, "PG_VERSION")) {
		progress("首次运行：初始化本地 PostgreSQL 数据目录（只在本机，不影响已安装的数据库）")
		if err := os.MkdirAll(service.data, 0755); err != nil {
			return err
		}
		passwordFile := filepath.Join(service.logs, "pgpass.txt")
		if err := os.WriteFile(passwordFile, []byte(service.superPassword), 0600); err != nil {
			return err
		}
		if err := service.runLogged(ctx, "initdb.log", "initdb",
			"-D", service.data, "-U", pgSuperUser, "--pwfile="+passwordFile,
			"-E", "UTF8", "-A", "scram-sha-256", "--locale=C"); err != nil {
			return fmt.Errorf("initdb 失败：%w", err)
		}
	}
	ctx, cancel := context.WithTimeout(ctx, listenTimeout)
	defer cancel()
	progress(fmt.Sprintf("启动本地 PostgreSQL（端口 %d）", service.port))
	if err := service.pgctl(ctx, "-D", service.data, "-l", filepath.Join(service.logs, "postgres.log"),
		"-o", fmt.Sprintf("-p %d -h 127.0.0.1", service.port), "-w", "start"); err != nil {
		return fmt.Errorf("PostgreSQL 启动失败：%w", err)
	}
	for start := time.Now(); !tcpOpen(service.port); {
		if time.Since(start) > listenTimeout {
			return fmt.Errorf("PostgreSQL 启动后未监听端口 %d", service.port)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("等待 PostgreSQL 启动时被取消")
		case <-time.After(300 * time.Millisecond):
		}
	}
	service.startMu.Lock()
	service.started = true
	service.startMu.Unlock()
	return nil
}

// ensureDatabase mirrors Ensure-Database in scripts/dev-env.ps1: the role and
// database exist with the launcher's stored password, whether the cluster is
// ours or one that already answered on the port.
func (service *postgresService) ensureDatabase(ctx context.Context, progress func(string)) error {
	ctx, cancel := context.WithTimeout(ctx, initTimeout)
	defer cancel()
	role := fmt.Sprintf("DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = '%s') THEN "+
		"CREATE ROLE \"%s\" LOGIN PASSWORD '%s'; "+
		"ELSE ALTER ROLE \"%s\" WITH LOGIN PASSWORD '%s'; END IF; END $$;",
		appDBUser, appDBUser, service.appPassword, appDBUser, service.appPassword)
	if _, err := service.psqlOutput(ctx, role); err != nil {
		return fmt.Errorf("无法配置应用数据库账号：%w", err)
	}
	output, err := service.psqlOutput(ctx, fmt.Sprintf("SELECT 1 FROM pg_database WHERE datname = '%s'", appDBName))
	if err != nil {
		return fmt.Errorf("无法查询应用数据库：%w", err)
	}
	if trimOutput([]byte(output)) != "1" {
		progress(fmt.Sprintf("创建数据库 %s", appDBName))
		if _, err := service.psqlOutput(ctx, fmt.Sprintf("CREATE DATABASE \"%s\" OWNER \"%s\"", appDBName, appDBUser)); err != nil {
			return fmt.Errorf("无法创建应用数据库：%w", err)
		}
	}
	if _, err := service.psqlOutput(ctx, fmt.Sprintf("ALTER DATABASE \"%s\" OWNER TO \"%s\"", appDBName, appDBUser)); err != nil {
		return fmt.Errorf("无法设置应用数据库所有者：%w", err)
	}
	return nil
}

func (service *postgresService) stop() {
	service.startMu.Lock()
	if !service.started {
		service.startMu.Unlock()
		return
	}
	service.started = false
	service.startMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = service.pgctl(ctx, "-D", service.data, "-m", "fast", "-w", "stop")
}

func trimOutput(output []byte) string {
	text := string(output)
	if len(text) > 2000 {
		text = text[:2000]
	}
	return text
}
