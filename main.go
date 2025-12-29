

// ==================================================
//                 IMPORTS
// ==================================================

import (
 "context"
 "fmt"
 "log"
 "net"
 "os"
 "os/exec"
 "os/signal"
 "runtime"
 "sort"
 "sync"
 "syscall"
 "time"

 "github.com/fatih/color"
)

// ==================================================
//                 CONSTANTS
// ==================================================

const (
 BaseSocksPort   = 9100
 BaseControlPort = 9200
 BalancerPort    = 10000
 DataDirBase     = "/var/lib/mojenx-tor"
 RotationEvery   = 5 * time.Minute
)

// ==================================================
//                 LOGGER
// ==================================================

var (
 logInfo    = color.New(color.FgCyan).PrintfFunc()
 logWarn    = color.New(color.FgYellow).PrintfFunc()
 logError   = color.New(color.FgRed).PrintfFunc()
 logSuccess = color.New(color.FgGreen).PrintfFunc()
)

// ==================================================
//                 MODELS
// ==================================================

type TorInstance struct {
 ID          int
 SocksPort   int
 ControlPort int
 DataDir     string
 Cmd         *exec.Cmd
 Alive       bool
 ConnCount   int
 mu          sync.Mutex
}

type Manager struct {
 Instances []*TorInstance
 Ctx       context.Context
 Cancel    context.CancelFunc
}

// ==================================================
//                 TOR INSTANCE
// ==================================================

func (t *TorInstance) Start() error {
 t.Cmd = exec.Command(
  "tor",
  "--SocksPort", fmt.Sprintf("%d", t.SocksPort),
  "--ControlPort", fmt.Sprintf("%d", t.ControlPort),
  "--DataDirectory", t.DataDir,
 )

 t.Cmd.Stdout = os.Stdout
 t.Cmd.Stderr = os.Stderr

 err := t.Cmd.Start()
 if err != nil {
  return err
 }

 t.Alive = true
 logSuccess("[Tor %d] started on SOCKS %d\n", t.ID, t.SocksPort)
 return nil
}

func (t *TorInstance) Watch(m *Manager) {
 go func() {
  err := t.Cmd.Wait()
  if err != nil {
   logWarn("[Tor %d] crashed, restarting...\n", t.ID)
  }
  t.Alive = false
  time.Sleep(2 * time.Second)
  _ = t.Start()
  t.Watch(m)
 }()
}

// ==================================================
//                 CIRCUIT ROTATION
// ==================================================

func (t *TorInstance) Rotate() {
 conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", t.ControlPort))
 if err != nil {
  logWarn("[Tor %d] control connect failed\n", t.ID)
  return
 }
 defer conn.Close()

 fmt.Fprintf(conn, "AUTHENTICATE \"\"\r\nSIGNAL NEWNYM\r\n")
 logInfo("[Tor %d] NEWNYM triggered\n", t.ID)
}

// ==================================================
//                 MANAGER
// ==================================================

func NewManager() *Manager {
 ctx, cancel := context.WithCancel(context.Background())
 return &Manager{
  Ctx:    ctx,
  Cancel: cancel,
 }
}

func (m *Manager) InitInstances() {
 count := runtime.NumCPU()
 logInfo("Detected %d CPU cores, spawning %d Tor instances\n", count, count)

 for i := 0; i < count; i++ {
  inst := &TorInstance{
   ID:          i + 1,
   SocksPort:   BaseSocksPort + i + 1,
   ControlPort: BaseControlPort + i + 1,
   DataDir:     fmt.Sprintf("%s/%d", DataDirBase, i+1),
  }
  _ = os.MkdirAll(inst.DataDir, 0700)
  m.Instances = append(m.Instances, inst)
 }
}

func (m *Manager) StartAll() {
 for _, inst := range m.Instances {
  if err := inst.Start(); err == nil {
   inst.Watch(m)
  }
 }
}

func (m *Manager) AutoRotate() {
 go func() {
  ticker := time.NewTicker(RotationEvery)
  defer ticker.Stop()

Moein, [12/29/2025 3:25 PM]
  for {
   select {
   case <-ticker.C:
    for _, inst := range m.Instances {
     inst.Rotate()
    }
   case <-m.Ctx.Done():
    return
   }
  }
 }()
}

// ==================================================
//                 LOAD BALANCER
// ==================================================

func (m *Manager) pickLeastConn() *TorInstance {
 sort.Slice(m.Instances, func(i, j int) bool {
  return m.Instances[i].ConnCount < m.Instances[j].ConnCount
 })
 return m.Instances[0]
}

func (m *Manager) StartBalancer() {
 ln, err := net.Listen("tcp", fmt.Sprintf(":%d", BalancerPort))
 if err != nil {
  log.Fatalf("Balancer failed: %v", err)
 }
 logSuccess("Balancer listening on :%d\n", BalancerPort)

 for {
  conn, err := ln.Accept()
  if err != nil {
   continue
  }

  go func(c net.Conn) {
   inst := m.pickLeastConn()
   inst.mu.Lock()
   inst.ConnCount++
   inst.mu.Unlock()

   defer func() {
    inst.mu.Lock()
    inst.ConnCount--
    inst.mu.Unlock()
   }()

   target, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", inst.SocksPort))
   if err != nil {
    logError("Dial tor failed\n")
    c.Close()
    return
   }

   go ioCopy(target, c)
   go ioCopy(c, target)
  }(conn)
 }
}

func ioCopy(dst net.Conn, src net.Conn) {
 defer dst.Close()
 defer src.Close()
 buf := make([]byte, 32*1024)
 for {
  n, err := src.Read(buf)
  if err != nil {
   return
  }
  dst.Write(buf[:n])
 }
}

// ==================================================
//                 ASCII LOGO
// ==================================================

func printLogo() {
 color.New(color.FgMagenta).Println(
███╗   ███╗ ██████╗      ██╗███████╗███╗   ██╗██╗  ██╗
████╗ ████║██╔═══██╗     ██║██╔════╝████╗  ██║╚██╗██╔╝
██╔████╔██║██║   ██║     ██║█████╗  ██╔██╗ ██║ ╚███╔╝
██║╚██╔╝██║██║   ██║██   ██║██╔══╝  ██║╚██╗██║ ██╔██╗
██║ ╚═╝ ██║╚██████╔╝╚█████╔╝███████╗██║ ╚████║██╔╝ ██╗
╚═╝     ╚═╝ ╚═════╝  ╚════╝ ╚══════╝╚═╝  ╚═══╝╚═╝  ╚═╝
          MojenX Tor - Multi Instance Manager
)
}

// ==================================================
//                 MAIN
// ==================================================

func main() {
 printLogo()

 manager := NewManager()
 manager.InitInstances()
 manager.StartAll()
 manager.AutoRotate()

 go manager.StartBalancer()

 sig := make(chan os.Signal, 1)
 signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
 <-sig

 logWarn("Shutting down MojenX Tor...\n")
 manager.Cancel()
 time.Sleep(1 * time.Second)
}

