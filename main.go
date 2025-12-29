package main

import (
 "context"
 "fmt"
 "io"
 "net"
 "os"
 "os/exec"
 "os/signal"
 "runtime"
 "sort"
 "sync"
 "syscall"
 "time"
)

//////////////////////////////////////////////////
// CONFIG
//////////////////////////////////////////////////

const (
 BaseSocksPort   = 9100
 BaseControlPort = 9200
 BalancerPort    = 10000

 DataDirBase = "/var/lib/mojenx-tor"

 RotateInterval  = 5 * time.Minute
 LatencyTimeout  = 3 * time.Second
 LatencyLimitMS  = 1500
 HealthCheckFreq = 30 * time.Second
)

//////////////////////////////////////////////////
// LOGGER
//////////////////////////////////////////////////

func logInfo(f string, a ...any)  { fmt.Printf("[INFO] "+f+"\n", a...) }
func logWarn(f string, a ...any)  { fmt.Printf("[WARN] "+f+"\n", a...) }
func logErr(f string, a ...any)   { fmt.Printf("[ERR ] "+f+"\n", a...) }
func logOK(f string, a ...any)    { fmt.Printf("[ OK ] "+f+"\n", a...) }

//////////////////////////////////////////////////
// MODELS
//////////////////////////////////////////////////

type TorInstance struct {
 ID          int
 SocksPort   int
 ControlPort int
 DataDir     string

 Cmd       *exec.Cmd
 Alive     bool
 ConnCount int
 LatencyMS int64

 mu sync.Mutex
}

type Manager struct {
 Ctx       context.Context
 Cancel    context.CancelFunc
 Instances []*TorInstance
}

//////////////////////////////////////////////////
// TOR INSTANCE
//////////////////////////////////////////////////

func (t *TorInstance) Start() error {
 cmd := exec.Command(
  "tor",
  "--SocksPort", fmt.Sprintf("%d", t.SocksPort),
  "--ControlPort", fmt.Sprintf("%d", t.ControlPort),
  "--DataDirectory", t.DataDir,
 )

 cmd.Stdout = os.Stdout
 cmd.Stderr = os.Stderr

 if err := cmd.Start(); err != nil {
  return err
 }

 t.Cmd = cmd
 t.Alive = true
 logOK("Tor #%d started (SOCKS %d)", t.ID, t.SocksPort)
 return nil
}

func (t *TorInstance) Watch() {
 go func() {
  err := t.Cmd.Wait()
  t.Alive = false
  if err != nil {
   logWarn("Tor #%d crashed, restarting...", t.ID)
  }
  time.Sleep(2 * time.Second)
  _ = t.Start()
  t.Watch()
 }()
}

func (t *TorInstance) Rotate() {
 conn, err := net.DialTimeout(
  "tcp",
  fmt.Sprintf("127.0.0.1:%d", t.ControlPort),
  2*time.Second,
 )
 if err != nil {
  return
 }
 defer conn.Close()

 fmt.Fprintf(conn, "AUTHENTICATE \"\"\r\nSIGNAL NEWNYM\r\n")
 logInfo("Tor #%d rotated circuit", t.ID)
}

//////////////////////////////////////////////////
// MANAGER
//////////////////////////////////////////////////

func NewManager() *Manager {
 ctx, cancel := context.WithCancel(context.Background())
 return &Manager{Ctx: ctx, Cancel: cancel}
}

func (m *Manager) Init() {
 count := runtime.NumCPU()
 logInfo("Detected %d CPU cores", count)

 for i := 0; i < count; i++ {
  inst := &TorInstance{
   ID:          i + 1,
   SocksPort:   BaseSocksPort + i + 1,
   ControlPort: BaseControlPort + i + 1,
   DataDir:     fmt.Sprintf("%s/%d", DataDirBase, i+1),
  }
  os.MkdirAll(inst.DataDir, 0700)
  m.Instances = append(m.Instances, inst)
 }
}

func (m *Manager) StartAll() {
 for _, t := range m.Instances {
  if err := t.Start(); err == nil {
   t.Watch()
  }
 }
}

//////////////////////////////////////////////////
// ROTATION + HEALTH
//////////////////////////////////////////////////

func (m *Manager) AutoRotate() {
 go func() {
  t := time.NewTicker(RotateInterval)
  for {
   select {
   case <-t.C:
    for _, i := range m.Instances {
     i.Rotate()
    }
   case <-m.Ctx.Done():
    return
   }
  }
 }()
}

func (m *Manager) HealthCheck() {
 go func() {
  t := time.NewTicker(HealthCheckFreq)
  for {
   select {
   case <-t.C:
    for _, i := range m.Instances {
     go checkLatency(i)
    }
   case <-m.Ctx.Done():
    return
   }
  }
 }()
}

func checkLatency(t *TorInstance) {
 start := time.Now()

 dialer := net.Dialer{Timeout: LatencyTimeout}
 conn, err := dialer.Dial(
  "tcp",
  fmt.Sprintf("127.0.0.1:%d", t.SocksPort),
 )
 if err != nil {
  return
 }
 conn.Close()

 ms := time.Since(start).Milliseconds()
 t.LatencyMS = ms

 if ms > LatencyLimitMS {
  logWarn("Tor #%d high latency (%dms), rotating", t.ID, ms)
  t.Rotate()
 }
}

//////////////////////////////////////////////////
// LOAD BALANCER
//////////////////////////////////////////////////

func (m *Manager) pick() *TorInstance {
 sort.Slice(m.Instances, func(i, j int) bool {
  return m.Instances[i].ConnCount < m.Instances[j].ConnCount
 })
 return m.Instances[0]
}

func (m *Manager) StartBalancer() {
 ln, err := net.Listen("tcp", fmt.Sprintf(":%d", BalancerPort))
 if err != nil {
  panic(err)
 }
 logOK("Balancer listening on :%d", BalancerPort)

 for {
  c, err := ln.Accept()
  if err != nil {
   continue
  }

  go m.handleConn(c)
 }
}

func (m *Manager) handleConn(c net.Conn) {
 inst := m.pick()

 inst.mu.Lock()
 inst.ConnCount++
 inst.mu.Unlock()

 defer func() {
  inst.mu.Lock()
  inst.ConnCount--
  inst.mu.Unlock()
 }()

 target, err := net.Dial(
  "tcp",
  fmt.Sprintf("127.0.0.1:%d", inst.SocksPort),
 )
 if err != nil {
  c.Close()
  return
 }

 go pipe(target, c)
 go pipe(c, target)
}

func pipe(dst, src net.Conn) {
 defer dst.Close()
 defer src.Close()
 io.Copy(dst, src)
}

//////////////////////////////////////////////////
// MAIN
//////////////////////////////////////////////////

func main() {
 fmt.Println("mojenx-tor | multi-instance tor manager")

 m := NewManager()
 m.Init()
 m.StartAll()
 m.AutoRotate()
 m.HealthCheck()

 go m.StartBalancer()

 sig := make(chan os.Signal, 1)
 signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
 <-sig

 logWarn("Shutting down...")
 m.Cancel()
 time.Sleep(time.Second)
}
