package server

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// CheckPortAvailable은 포트가 사용 가능한지 확인합니다.
func CheckPortAvailable(port string) error {
	address := ":" + port
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("포트 %s가 이미 사용 중입니다: %w", port, err)
	}
	listener.Close()
	return nil
}

// FindProcessUsingPort는 특정 포트를 사용하는 프로세스를 찾습니다.
func FindProcessUsingPort(port string) ([]string, error) {
	var cmd *exec.Cmd
	
	switch runtime.GOOS {
	case "darwin", "linux":
		// Unix 계열: lsof 사용
		cmd = exec.Command("lsof", "-ti", fmt.Sprintf(":%s", port))
	case "windows":
		// Windows: netstat 사용
		cmd = exec.Command("cmd", "/C", fmt.Sprintf("netstat -ano | findstr :%s", port))
	default:
		return nil, fmt.Errorf("지원하지 않는 OS: %s", runtime.GOOS)
	}
	
	output, err := cmd.Output()
	if err != nil {
		// 프로세스가 없으면 에러 반환 (정상)
		return nil, nil
	}
	
	pids := strings.Split(strings.TrimSpace(string(output)), "\n")
	return pids, nil
}

// KillProcessByPID는 PID로 프로세스를 종료합니다.
func KillProcessByPID(pid string) error {
	pidInt, err := strconv.Atoi(strings.TrimSpace(pid))
	if err != nil {
		return fmt.Errorf("잘못된 PID: %s", pid)
	}
	
	process, err := os.FindProcess(pidInt)
	if err != nil {
		return fmt.Errorf("프로세스를 찾을 수 없음 (PID: %d): %w", pidInt, err)
	}
	
	// SIGTERM 전송 (graceful shutdown 시도)
	if err := process.Signal(os.Interrupt); err != nil {
		// SIGKILL로 강제 종료
		return process.Kill()
	}
	
	return nil
}

// EnsurePortAvailable은 포트를 사용 가능하게 만듭니다.
// 기존 프로세스가 있으면 사용자에게 확인 후 종료합니다.
func EnsurePortAvailable(port string, autoKill bool) error {
	// 1. 포트 사용 가능 여부 확인
	if err := CheckPortAvailable(port); err == nil {
		// 포트가 사용 가능함
		return nil
	}
	
	log.Printf("⚠️  포트 %s가 이미 사용 중입니다.", port)
	
	// 2. 포트를 사용하는 프로세스 찾기
	pids, err := FindProcessUsingPort(port)
	if err != nil {
		log.Printf("⚠️  프로세스 검색 실패: %v", err)
		return fmt.Errorf("포트 %s를 사용하는 프로세스를 찾을 수 없습니다", port)
	}
	
	if len(pids) == 0 {
		// TIME_WAIT 상태일 수 있음 - 잠시 대기
		log.Println("포트가 TIME_WAIT 상태일 수 있습니다. 5초 대기 중...")
		time.Sleep(5 * time.Second)
		
		// 재확인
		if err := CheckPortAvailable(port); err == nil {
			log.Println("✅ 포트가 사용 가능해졌습니다.")
			return nil
		}
		
		return fmt.Errorf("포트 %s를 여전히 사용할 수 없습니다", port)
	}
	
	// 3. 프로세스 종료
	log.Printf("포트 %s를 사용하는 프로세스: %v", port, pids)
	
	if !autoKill {
		log.Println("다음 명령으로 수동으로 종료하세요:")
		for _, pid := range pids {
			if runtime.GOOS == "windows" {
				log.Printf("  taskkill /F /PID %s", pid)
			} else {
				log.Printf("  kill -9 %s", pid)
			}
		}
		return fmt.Errorf("기존 프로세스를 먼저 종료해주세요")
	}
	
	// 자동 종료
	log.Println("기존 프로세스를 종료합니다...")
	for _, pid := range pids {
		if err := KillProcessByPID(pid); err != nil {
			log.Printf("⚠️  PID %s 종료 실패: %v", pid, err)
		} else {
			log.Printf("✅ PID %s 종료 완료", pid)
		}
	}
	
	// 프로세스 종료 대기
	time.Sleep(2 * time.Second)
	
	// 최종 확인
	if err := CheckPortAvailable(port); err != nil {
		return fmt.Errorf("프로세스를 종료했지만 여전히 포트를 사용할 수 없습니다: %w", err)
	}
	
	log.Printf("✅ 포트 %s가 사용 가능합니다.", port)
	return nil
}

