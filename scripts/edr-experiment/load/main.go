// load: controlled syscall/ES-event workload generator. usage: load <mode> <seconds> <rate/s> <falconpid>
package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var work = "/tmp/edr-exp/work"

func cpuSecs(pid string) float64 { // cumulative CPU seconds of pid via ps cputime (10ms res)
	out, err := exec.Command("ps", "-o", "cputime=", "-p", pid).Output()
	if err != nil {
		return 0
	}
	s := strings.TrimSpace(string(out))
	parts := strings.Split(s, ":")
	var t float64
	for _, p := range parts {
		v, _ := strconv.ParseFloat(p, 64)
		t = t*60 + v
	}
	return t
}

func main() {
	mode, dur, rate, fpid := os.Args[1], 0, 0, os.Args[4]
	dur, _ = strconv.Atoi(os.Args[2])
	rate, _ = strconv.Atoi(os.Args[3])
	op := ops(mode)
	if op == nil {
		fmt.Println("unknown mode", mode)
		os.Exit(2)
	}
	c0 := cpuSecs(fpid)
	sp0 := cpuSecs(os.Getenv("SPID"))
	t0 := time.Now()
	end := t0.Add(time.Duration(dur) * time.Second)
	var lat []time.Duration
	n := 0
	var interval time.Duration
	if rate > 0 {
		interval = time.Second / time.Duration(rate)
	}
	next := t0
	for time.Now().Before(end) {
		if rate > 0 {
			if d := time.Until(next); d > 0 {
				time.Sleep(d)
			}
			next = next.Add(interval)
			if time.Since(next) > time.Second { // fell behind; don't burst
				next = time.Now()
			}
		}
		s := time.Now()
		op(n)
		if n%1 == 0 {
			lat = append(lat, time.Since(s))
		}
		n++
	}
	el := time.Since(t0).Seconds()
	c1 := cpuSecs(fpid)
	sp1 := cpuSecs(os.Getenv("SPID"))
	sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
	pct := func(p float64) float64 {
		if len(lat) == 0 {
			return 0
		}
		return float64(lat[int(float64(len(lat)-1)*p)].Microseconds()) / 1000
	}
	fmt.Printf("RESULT mode=%s ops=%d rate=%.1f/s falcon_cpu_pct=%.1f syspolicyd_cpu_pct=%.1f lat_p50_ms=%.3f lat_p95_ms=%.3f\n", mode, n, float64(n)/el, (c1-c0)/el*100, (sp1-sp0)/el*100, pct(.5), pct(.95))
}

func run(name string, args ...string) func(int) {
	return func(int) { _ = exec.Command(name, args...).Run() }
}

func ops(mode string) func(int) {
	_ = os.MkdirAll(work, 0o755)
	switch mode {
	case "cheap": // no ES events
		return func(int) { syscall.Getpid(); _ = time.Now() }
	case "readfd": // pread on already-open fd
		f, _ := os.Create(work + "/readfd.bin")
		f.Write(make([]byte, 4096))
		b := make([]byte, 4096)
		return func(int) { f.ReadAt(b, 0) }
	case "createwrite":
		d := work + "/cw"
		os.RemoveAll(d)
		os.MkdirAll(d, 0o755)
		return func(i int) { os.WriteFile(fmt.Sprintf("%s/f%d", d, i%2000), []byte("hello world\n"), 0o644) }
	case "openread":
		d := work + "/or"
		os.MkdirAll(d, 0o755)
		for i := 0; i < 200; i++ {
			os.WriteFile(fmt.Sprintf("%s/f%d", d, i), []byte("hello world\n"), 0o644)
		}
		b := make([]byte, 64)
		return func(i int) {
			f, err := os.Open(fmt.Sprintf("%s/f%d", d, i%200))
			if err == nil {
				f.Read(b)
				f.Close()
			}
		}
	case "renameunlink":
		d := work + "/ru"
		os.RemoveAll(d)
		os.MkdirAll(d, 0o755)
		return func(i int) {
			a, b := fmt.Sprintf("%s/a%d", d, i), fmt.Sprintf("%s/b%d", d, i)
			os.WriteFile(a, []byte("x"), 0o644)
			os.Rename(a, b)
			os.Remove(b)
		}
	case "stat": // lstat storm over a tree (one op = 1 lstat)
		var paths []string
		filepath.WalkDir(os.Getenv("STATROOT"), func(p string, d os.DirEntry, err error) error {
			if len(paths) < 50000 {
				paths = append(paths, p)
			}
			return nil
		})
		return func(i int) { os.Lstat(paths[i%len(paths)]) }
	case "uds", "tcp":
		var l net.Listener
		var c net.Conn
		if mode == "uds" {
			os.Remove(work + "/s.sock")
			l, _ = net.Listen("unix", work+"/s.sock")
		} else {
			l, _ = net.Listen("tcp", "127.0.0.1:0")
		}
		go func() {
			s, _ := l.Accept()
			b := make([]byte, 64)
			for {
				n, err := s.Read(b)
				if err != nil {
					return
				}
				s.Write(b[:n])
			}
		}()
		if mode == "uds" {
			c, _ = net.Dial("unix", work+"/s.sock")
		} else {
			c, _ = net.Dial("tcp", l.Addr().String())
		}
		b := make([]byte, 64)
		return func(int) { c.Write([]byte("ping")); c.Read(b) }
	// ---- exec family
	case "true":
		return run("/usr/bin/true")
	case "sh":
		return run("/bin/sh", "-c", "true")
	case "bash":
		return run("/bin/bash", "-c", "true")
	case "zsh":
		return run("/bin/zsh", "-c", "true")
	case "zshl":
		return run("/bin/zsh", "-l", "-c", "true")
	case "zshi":
		return run("/bin/zsh", "-i", "-c", "true")
	case "shmany": // one shell, 20 builtins
		return run("/bin/bash", "-c", strings.Repeat("true; ", 20))
	case "shebang_cached":
		p := work + "/cached.sh"
		os.WriteFile(p, []byte("#!/bin/sh\ntrue\n"), 0o755)
		return run(p)
	case "shebang_fresh":
		return func(i int) {
			p := fmt.Sprintf("%s/fresh%d.sh", work, i)
			os.WriteFile(p, []byte("#!/bin/sh\ntrue # "+strconv.Itoa(i)+"\n"), 0o755)
			exec.Command(p).Run()
			os.Remove(p)
		}
	case "py":
		return run("python3", "-c", "pass")
	case "pyabs":
		return run("/usr/bin/python3", "-c", "pass")
	case "node":
		return run("node", "-e", "0")
	case "bun":
		return run("bun", "-e", "0")
	case "bin_cached": // copy once, exec many (small go binary: this program itself, mode that exits)
		self, _ := os.Executable()
		p := work + "/cachedbin"
		cp(self, p)
		return run(p, "noop", "0", "0", "1")
	case "bin_fresh": // new inode every exec
		self, _ := os.Executable()
		return func(i int) {
			p := fmt.Sprintf("%s/freshbin%d", work, i)
			cp(self, p)
			exec.Command(p, "noop", "0", "0", "1").Run()
			os.Remove(p)
		}
	case "big_cached":
		p := work + "/bigcached"
		cp(os.Getenv("BIGBIN"), p)
		return run(p, "--version")
	case "big_fresh":
		return func(i int) {
			p := fmt.Sprintf("%s/bigfresh%d", work, i)
			cp(os.Getenv("BIGBIN"), p)
			exec.Command(p, "--version").Run()
			os.Remove(p)
		}
	case "shebang_fresh_same": // new path + new inode, identical content every time
		return func(i int) {
			p := fmt.Sprintf("%s/fs%d.sh", work, i)
			os.WriteFile(p, []byte("#!/bin/sh\ntrue\n"), 0o755)
			exec.Command(p).Run()
			os.Remove(p)
		}
	case "shebang_rewrite": // same path rewritten with identical bytes, then exec
		p := work + "/rewrite.sh"
		return func(i int) {
			os.WriteFile(p, []byte("#!/bin/sh\ntrue\n"), 0o755)
			exec.Command(p).Run()
		}
	case "true_copy_fresh": // Apple-signed /usr/bin/true copied to a new path each time
		return func(i int) {
			p := fmt.Sprintf("%s/tc%d", work, i)
			cp("/usr/bin/true", p)
			exec.Command(p).Run()
			os.Remove(p)
		}
	case "writeonly_fresh": // control: write the same fresh script but never exec it
		return func(i int) {
			p := fmt.Sprintf("%s/wo%d.sh", work, i)
			os.WriteFile(p, []byte("#!/bin/sh\ntrue\n"), 0o755)
			os.Remove(p)
		}
	case "pwrite": // sqlite-like: 4KB pwrite at random offsets into a pre-existing 256MB file, fsync every 256 writes
		f, _ := os.OpenFile(work+"/pw.db", os.O_RDWR|os.O_CREATE, 0o644)
		if st, _ := f.Stat(); st.Size() < 256<<20 {
			buf := make([]byte, 1<<20)
			for i := 0; i < 256; i++ {
				f.WriteAt(buf, int64(i)<<20)
			}
			f.Sync()
		}
		pg := make([]byte, 4096)
		return func(i int) {
			f.WriteAt(pg, int64((i*2654435761)%65536)*4096)
			if i%256 == 255 {
				f.Sync()
			}
		}
	case "noop":
		return func(int) {}
	case "sleep": // quiet baseline
		return func(int) { time.Sleep(100 * time.Millisecond) }
	}
	return nil
}

func cp(src, dst string) {
	b, err := os.ReadFile(src)
	if err != nil {
		return
	}
	os.WriteFile(dst, b, 0o755)
}
