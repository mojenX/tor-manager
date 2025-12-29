package main

import (
 "bufio"
 "bytes"
 "fmt"
 "net"
 "os"
 "os/exec"
 "strings"
 "time"
)

//////////////////////////////////////////////////
// CONFIG
//////////////////////////////////////////////////

const (
 TOR_SERVICE  = "tor"
 SOCKS_ADDR   = "127.0.0.1:9050"
 CTRL_ADDR    = "127.0.0.1:9051"
 TORRC_PATH   = "/etc/tor/torrc"
 CRON_TAG     = "#MOJENX_ROTATE"
 CHECK_IP_URL = "https://api.ipify.org"
 LOG_FILE     = "/var/log/mojenx-tor.log"
)

//////////////////////////////////////////////////
// LOGGER
//////////////////////////////////////////////////

func logLine(level, msg string) {
 line := fmt.Sprintf("[%s] [%s] %s\n",
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

func logInfo(m string) { logLine("INFO", m) }
func logWarn(m string) { logLine("WARN", m) }
func logErr(m string)  { logLine("ERROR", m) }

//////////////////////////////////////////////////
// SYSTEM UTILS
//////////////////////////////////////////////////

func runCmd(name string, args ...string) (string, error) {
 cmd := exec.Command(name, args...)
 var out bytes.Buffer
 cmd.Stdout = &out
 cmd.Stderr = &out
 err := cmd.Run()
 return out.String(), err
}

func mustRoot() {
 if os.Geteuid() != 0 {
  fmt.Println("Run as root")
  os.Exit(1)
 }
}

//////////////////////////////////////////////////
// TOR CONTROL
//////////////////////////////////////////////////

func torInstalled() bool {
 _, err := exec.LookPath("tor")
 return err == nil
}

func torStart()   { runCmd("systemctl", "start", TOR_SERVICE) }
func torStop()    { runCmd("systemctl", "stop", TOR_SERVICE) }
func torRestart() { runCmd("systemctl", "restart", TOR_SERVICE) }

func torStatus() string {
 out, _ := runCmd("systemctl", "is-active", TOR_SERVICE)
 return strings.TrimSpace(out)
}

//////////////////////////////////////////////////
// TOR IP
//////////////////////////////////////////////////

func socksAlive() bool {
 c, err := net.DialTimeout("tcp", SOCKS_ADDR, 2*time.Second)
 if err != nil {
  return false
 }
 c.Close()
 return true
}

func getTorIP() string {
 if !socksAlive() {
  return "Tor not running"
 }
 cmd := exec.Command("curl", "-s", "--socks5-hostname", SOCKS_ADDR, CHECK_IP_URL)
 out, err := cmd.Output()
 if err != nil {
  return "Error"
 }
 return strings.TrimSpace(string(out))
}

//////////////////////////////////////////////////
// NEWNYM
//////////////////////////////////////////////////

func rotateIP() {
 c, err := net.Dial("tcp", CTRL_ADDR)
 if err != nil {
  fmt.Println("ControlPort not reachable")
  return
 }
 defer c.Close()
 fmt.Fprintf(c, "AUTHENTICATE \"\"\r\nSIGNAL NEWNYM\r\n")
 logInfo("NEWNYM sent")
}

//////////////////////////////////////////////////
// TORRC
//////////////////////////////////////////////////

func setCountry(code string) {
 data, _ := os.ReadFile(TORRC_PATH)
 lines := strings.Split(string(data), "\n")

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

 os.WriteFile(TORRC_PATH, []byte(strings.Join(out, "\n")), 0644)
 torRestart()
 logInfo("Exit country set to " + code)
}

//////////////////////////////////////////////////
// CRON
//////////////////////////////////////////////////

func setAutoRotate(min string) {
 line := "*/" + min + " * * * * printf \"AUTHENTICATE \\\"\\\"\\r\\nSIGNAL NEWNYM\\r\\n\" | nc 127.0.0.1 9051 " + CRON_TAG
 old, _ := runCmd("crontab", "-l")

 var out []string
 for _, l := range strings.Split(old, "\n") {
  if !strings.Contains(l, CRON_TAG) {
   out = append(out, l)
  }
 }
 out = append(out, line)

 cmd := exec.Command("crontab", "-")
 cmd.Stdin = strings.NewReader(strings.Join(out, "\n"))
 cmd.Run()

 logInfo("Auto rotate set every " + min + " minutes")
}

//////////////////////////////////////////////////
// UI
//////////////////////////////////////////////////

func clear() {
 fmt.Print("\033[H\033[2J")
}

func pause() {
 fmt.Print("\nPress ENTER...")
 bufio.NewReader(os.Stdin).ReadBytes('\n')
}

func banner() {
 fmt.Println("===================================")
 fmt.Println("        MOJENX TOR MANAGER          ")
 fmt.Println("===================================")
}

//////////////////////////////////////////////////
// MENU
//////////////////////////////////////////////////

func menu() {
 r := bufio.NewReader(os.Stdin)

 for {
  clear()
  banner()

  fmt.Println("Tor Installed :", torInstalled())
  fmt.Println("Tor Status    :", torStatus())
  fmt.Println("SOCKS Alive   :", socksAlive())
  fmt.Println("Current IP    :", getTorIP())
  fmt.Println("-----------------------------------")
  fmt.Println("1) Start Tor")
  fmt.Println("2) Stop Tor")
  fmt.Println("3) Restart Tor")
  fmt.Println("4) Show Tor Status")
  fmt.Println("5) Show IP")
  fmt.Println("6) Rotate IP")
  fmt.Println("7) Set Exit Country")
  fmt.Println("8) Set Auto Rotate")
  fmt.Println("0) Exit")
  fmt.Print("\nSelect: ")

  c, _ := r.ReadString('\n')
  c = strings.TrimSpace(c)

  switch c {
  case "1":
   torStart()
  case "2":
   torStop()
  case "3":
   torRestart()
  case "4":
   fmt.Println("Status:", torStatus())
  case "5":
   fmt.Println("IP:", getTorIP())
  case "6":
   rotateIP()
  case "7":
   fmt.Print("Country code: ")
   x, _ := r.ReadString('\n')
   setCountry(strings.TrimSpace(x))
  case "8":
   fmt.Print("Minutes: ")
   m, _ := r.ReadString('\n')
   setAutoRotate(strings.TrimSpace(m))
  case "0":
   os.Exit(0)
  default:
   fmt.Println("Invalid choice")
  }
  pause()
 }
}

//////////////////////////////////////////////////
// MAIN
//////////////////////////////////////////////////

func main() {
 mustRoot()
 menu()
}
