//go:build !windows

package manager

import (
	"bufio"
	"errors"
	"os"
	"strconv"
	"strings"
)

func detectPhysicalMemoryGB() (int, error) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 && fields[0] == "MemTotal:" {
			kilobytes, parseErr := strconv.ParseUint(fields[1], 10, 64)
			if parseErr != nil {
				return 0, parseErr
			}
			gigabytes := int(kilobytes / (1024 * 1024))
			if gigabytes < 1 {
				gigabytes = 1
			}
			return gigabytes, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return 0, errors.New("MemTotal não encontrado em /proc/meminfo")
}
