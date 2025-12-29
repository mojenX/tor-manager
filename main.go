package main

import (
 "bufio"
 "bytes"
 "fmt"
 "net"
 "os"
 "os/exec"
 "strings"
 "sync"
 "time"
)

//////////////////////////////////////////////////
// CONSTANTS
//////////////////////////////////////////////////

const (
 TOR_SERVICE  = "tor"
 SOCKS_ADDR   = "127.0.0.1:9050"
 CTRL_ADDR    = "127.0.0.1:9051"
 TORRC_PATH   = "/etc/tor/torrc"
 CHECK_IP_URL = "https://api.ipify.org"
 CRON_TAG     = "#MOJENX_TOR_ROTATE"
 LOG_FILE     = "/var/log/mojenx-tor.log"
)

//////////////////////////////////////////////////
// GLOBAL CACHE STATE
//////////////////////////////////////////////////

var (
 cacheMu        sync.Mutex
 cachedIP       = "unknown"
 cachedStatus   = "unknown"
 cachedSOCKS    = false
 lastIPUpdate   time.Time
 lastStatUpdate time.Time
)

//////////////////////////////////////////////////
// LOGGER
//////////////////////////////////////////////////

func logLine(level string, msg string) {
 line := fmt.Sprintf(
  "[%s] [%s] %s\n",
  time.Now().Format("2006-01-02 15:04:05"),
  level,
  msg,
 )

 fmt.Print(line)

 f, err := os.OpenFile(LOG_FILE, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
 if err == nil {
  f.WriteString(line)
  f.Close()
 }
}

func logInfo(msg string) { logLine("INFO", msg) }
func logWarn(msg string) { logLine("WARN", msg) }
func logErr(msg string)  { logLine("ERROR", msg) }

//////////////////////////////////////////////////
// BASIC UTILS
//////////////////////////////////////////////////

func mustRoot() {
 if os.Geteuid() != 0 {
  fmt.Println("Please run as root")
  os.Exit(1)
 }
}

func runCmd(name string, args ...string) (string, error) {
 cmd := exec.Command(name, args...)
 var out bytes.Buffer
 cmd.Stdout = &out
 cmd.Stderr = &out
 err := cmd.Run()
 return out.String(), err
}

//////////////////////////////////////////////////
// TERMINAL UI
//////////////////////////////////////////////////

func clearScreen() {
 fmt.Print("\033[H\033[2J")
}

func pause() {
 fmt.Print("\nPress ENTER to continue...")
 bufio.NewReader(os.Stdin).ReadBytes('\n')
}

func banner() {
 fmt.Println("======================================")
 fmt.Println("        MOJENX TOR MANAGER             ")
 fmt.Println("======================================")
}

//////////////////////////////////////////////////
// TOR SERVICE CONTROL
//////////////////////////////////////////////////

func torInstalled() bool {
 _, err := exec.LookPath("tor")
 return err == nil
}

func torStart() {
 logInfo("Starting Tor service")
 runCmd("systemctl", "start", TOR_SERVICE)
}

func torStop() {
 logInfo("Stopping Tor service")
 runCmd("systemctl", "stop", TOR_SERVICE)
}

func torRestart() {
 logInfo("Restarting Tor service")
 runCmd("systemctl", "restart", TOR_SERVICE)
}

func torStatusRaw() string {
 out, _ := runCmd("systemctl", "is-active", TOR_SERVICE)
 return strings.TrimSpace(out)
}

//////////////////////////////////////////////////
// SOCKS & IP (LOW LEVEL)
//////////////////////////////////////////////////

func socksAliveRaw() bool {
 conn, err := net.DialTimeout("tcp", SOCKS_ADDR, 2*time.Second)
 if err != nil {
  return false
 }
 conn.Close()
 return true
}

func fetchTorIP() string {
 if !socksAliveRaw() {
  return "Tor not running"
 }

 cmd := exec.Command(
  "curl",
  "-s",
  "--socks5-hostname", SOCKS_ADDR,
  CHECK_IP_URL,
 )

 out, err := cmd.Output()
 if err != nil {
  return "error"
 }

 return strings.TrimSpace(string(out))
}

//////////////////////////////////////////////////
// CACHE REFRESH (BACKGROUND)
//////////////////////////////////////////////////

func refreshCacheLoop() {
 for {
  cacheMu.Lock()

  if time.Since(lastStatUpdate) > 5*time.Second {
   cachedStatus = torStatusRaw()
   cachedSOCKS = socksAliveRaw()
   lastStatUpdate = time.Now()
  }

  if time.Since(lastIPUpdate) > 15*time.Second {
   cachedIP = fetchTorIP()
   lastIPUpdate = time.Now()
  }

  cacheMu.Unlock()

  time.Sleep(1 * time.Second)
 }
}

//////////////////////////////////////////////////
// CONTROL PORT (NEWNYM)
//////////////////////////////////////////////////

func rotateIP() {
 conn, err := net.Dial("tcp", CTRL_ADDR)
 if err != nil {
  fmt.Println("ControlPort not reachable")
  logErr("ControlPort not reachable")
  return
 }
 defer conn.Close()

 fmt.Fprintf(conn, "AUTHENTICATE \"\"\r\nSIGNAL NEWNYM\r\n")
 logInfo("NEWNYM signal sent")
 fmt.Println("Tor IP rotated")
}

//////////////////////////////////////////////////
// TORRC MANAGEMENT
//////////////////////////////////////////////////

func readTorrc() []string {
 data, _ := os.ReadFile(TORRC_PATH)
 return strings.Split(string(data), "\n")
}

func writeTorrc(lines []string) {
 os.WriteFile(TORRC_PATH, []byte(strings.Join(lines, "\n")), 0644)
}

func setExitCountry(code string) {
 lines := readTorrc()
 var out []string

 for _, l := range lines {
  if strings.HasPrefix(l, "ExitNodes") || strings.HasPrefix(l, "StrictNodes") {
   continue
  }
  out = append(out, l)
 }

 out = append(out,
  "ExitNodes {"+strings.ToUpper(code)+"}",
  "StrictNodes 1",
 )

 writeTorrc(out)
 logInfo("Exit country set to " + code)
 torRestart()
}

//////////////////////////////////////////////////
// CRON AUTO ROTATION
//////////////////////////////////////////////////

func setAutoRotate(minutes string) {
 line := "*/" + minutes +
  " * * * * printf \"AUTHENTICATE \\\"\\\"\\r\\nSIGNAL NEWNYM\\r\\n\" | nc 127.0.0.1 9051 " +
  CRON_TAG

 current, _ := runCmd("crontab", "-l")
 var out []string

 for _, l := range strings.Split(current, "\n") {
  if !strings.Contains(l, CRON_TAG) {
   out = append(out, l)
  }
 }
 out = append(out, line)

 cmd := exec.Command("crontab", "-")
 cmd.Stdin = strings.NewReader(strings.Join(out, "\n"))
 cmd.Run()

 logInfo("Auto rotate every " + minutes + " minutes")
}

//////////////////////////////////////////////////
// SAFE HEADER (NO FLICKER)
//////////////////////////////////////////////////

func showHeader() {
 cacheMu.Lock()
 defer cacheMu.Unlock()

 fmt.Println("Tor Installed :", torInstalled())
 fmt.Println("Tor Status    :", cachedStatus)
 fmt.Println("SOCKS Alive   :", cachedSOCKS)
 fmt.Println("Current IP    :", cachedIP)
}

//////////////////////////////////////////////////
// MENU
//////////////////////////////////////////////////

func menu() {
 reader := bufio.NewReader(os.Stdin)

 for {
  clearScreen()
  banner()
  showHeader()

  fmt.Println("--------------------------------------")
  fmt.Println("1) Start Tor")
  fmt.Println("2) Stop Tor")
  fmt.Println("3) Restart Tor")
  fmt.Println("4) Show Tor Status (live)")
  fmt.Println("5) Get Tor IP (live)")
  fmt.Println("6) Rotate IP (NEWNYM)")
  fmt.Println("7) Set Exit Country")
  fmt.Println("8) Set Auto Rotate (cron)")
  fmt.Println("0) Exit")
  fmt.Print("\nSelect option: ")

  choice, _ := reader.ReadString('\n')
  choice = strings.TrimSpace(choice)

  switch choice {

  case "1":
   torStart()

  case "2":
   torStop()

  case "3":
   torRestart()

  case "4":
   fmt.Println("Tor Status:", torStatusRaw())

  case "5":
   fmt.Println("Tor IP:", fetchTorIP())

  case "6":
   rotateIP()

  case "7":
   fmt.Print("Country code (DE, NL, FR...): ")
   c, _ := reader.ReadString('\n')
   setExitCountry(strings.TrimSpace(c))

  case "8":
   fmt.Print("Rotate every N minutes: ")
   m, _ := reader.ReadString('\n')
   setAutoRotate(strings.TrimSpace(m))

  case "0":
   logInfo("Exit requested")
   os.Exit(0)

  default:
   fmt.Println("Invalid option")
  }

  pause()
 }
}

//////////////////////////////////////////////////
// MAIN
//////////////////////////////////////////////////

func main() {
 mustRoot()
 go refreshCacheLoop()
 menu()
}
