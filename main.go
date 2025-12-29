package main

import (
 "bufio"
 "bytes"
 "fmt"
 "io"
 "net"
 "os"
 "os/exec"
 "strings"
 "time"
)

//////////////////////////////////////////////////
// GLOBAL CONFIG
//////////////////////////////////////////////////

const (
 TOR_SERVICE    = "tor"
 SOCKS_ADDR     = "127.0.0.1:9050"
 CONTROL_ADDR   = "127.0.0.1:9051"
 TORRC_PATH     = "/etc/tor/torrc"
 CRON_TAG       = "# MOJENX_TOR_ROTATE"
 CHECK_IP_URL   = "https://api.ipify.org"
 LOG_FILE       = "/var/log/mojenx-tor.log"
)

//////////////////////////////////////////////////
// LOGGER
//////////////////////////////////////////////////

func logLine(level, msg string) {
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

func logInfo(m string)  { logLine("INFO", m) }
func logWarn(m string)  { logLine("WARN", m) }
func logError(m string) { logLine("ERROR", m) }

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
 fmt.Println(
███╗   ███╗  ██████╗      ██╗███████╗███╗   ██╗██╗  ██╗
████╗ ████║ ██╔═══██╗     ██║██╔════╝████╗  ██║╚██╗██╔╝
██╔████╔██║ ██║   ██║     ██║█████╗  ██╔██╗ ██║ ╚███╔╝
██║╚██╔╝██║ ██║   ██║██   ██║██╔══╝  ██║╚██╗██║ ██╔██╗
██║ ╚═╝ ██║ ╚██████╔╝╚█████╔╝███████╗██║ ╚████║██╔╝ ██╗
╚═╝     ╚═╝  ╚═════╝  ╚════╝ ╚══════╝╚═╝  ╚═══╝╚═╝  ╚═╝
          MOJENX TOR MANAGER
)
}

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

func requireRoot() {
 if os.Geteuid() != 0 {
  fmt.Println("Run as root")
  os.Exit(1)
 }
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

func torStatus() string {
 out, _ := runCmd("systemctl", "is-active", TOR_SERVICE)
 return strings.TrimSpace(out)
}

//////////////////////////////////////////////////
// TOR IP / SOCKS
//////////////////////////////////////////////////

func torSocksAlive() bool {
 conn, err := net.DialTimeout("tcp", SOCKS_ADDR, 2*time.Second)
 if err != nil {
  return false
 }
 conn.Close()
 return true
}

func getTorIP() string {
 if !torSocksAlive() {
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
  return "Error"
 }
 return strings.TrimSpace(string(out))
}

//////////////////////////////////////////////////
// CONTROL PORT / NEWNYM
//////////////////////////////////////////////////

func rotateIP() {
 conn, err := net.Dial("tcp", CONTROL_ADDR)
 if err != nil {
  logError("ControlPort not reachable")
  fmt.Println("ControlPort not available")
  return
 }
 defer conn.Close()

Moein, [12/29/2025 4:38 PM]
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

func cleanTorrc() []string {
 var out []string
 for _, l := range readTorrc() {
  if strings.HasPrefix(l, "ExitNodes") ||
   strings.HasPrefix(l, "StrictNodes") {
   continue
  }
  out = append(out, l)
 }
 return out
}

func setExitCountry(code string) {
 lines := cleanTorrc()
 lines = append(lines,
  fmt.Sprintf("ExitNodes {%s}", strings.ToUpper(code)),
  "StrictNodes 1",
 )
 writeTorrc(lines)
 logInfo("Exit country set to " + code)
 torRestart()
}

//////////////////////////////////////////////////
// CRON MANAGEMENT
//////////////////////////////////////////////////

func setAutoRotate(minutes string) {
 entry := fmt.Sprintf(
  "*/%s * * * * printf \"AUTHENTICATE \\\"\\\"\\r\\nSIGNAL NEWNYM\\r\\n\" | nc 127.0.0.1 9051 %s",
  minutes,
  CRON_TAG,
 )

 current, _ := runCmd("crontab", "-l")
 var out []string
 for _, l := range strings.Split(current, "\n") {
  if !strings.Contains(l, CRON_TAG) {
   out = append(out, l)
  }
 }
 out = append(out, entry)

 cmd := exec.Command("crontab", "-")
 cmd.Stdin = strings.NewReader(strings.Join(out, "\n"))
 cmd.Run()

 logInfo("Auto rotate set every " + minutes + " minutes")
}

//////////////////////////////////////////////////
// MENU
//////////////////////////////////////////////////

func menu() {
 reader := bufio.NewReader(os.Stdin)

 for {
  clearScreen()
  banner()

  fmt.Println("Tor Installed :", torInstalled())
  fmt.Println("Tor Service   :", torStatus())
  fmt.Println("Tor SOCKS     :", torSocksAlive())
  fmt.Println("Current IP    :", getTorIP())
  fmt.Println("--------------------------------------")
  fmt.Println("1) Install Tor")
  fmt.Println("2) Start Tor")
  fmt.Println("3) Stop Tor")
  fmt.Println("4) Restart Tor")
  fmt.Println("5) Show Tor Status")
  fmt.Println("6) Show Tor IP")
  fmt.Println("7) Rotate IP (NEWNYM)")
  fmt.Println("8) Set Exit Country")
  fmt.Println("9) Set Auto Rotate (cron)")
  fmt.Println("0) Exit")
  fmt.Print("\nEnter choice: ")

  choice, _ := reader.ReadString('\n')
  choice = strings.TrimSpace(choice)

  switch choice {

  case "1":
   runCmd("apt", "install", "-y", "tor")

  case "2":
   torStart()

  case "3":
   torStop()

  case "4":
   torRestart()

  case "5":
   out, _ := runCmd("systemctl", "status", TOR_SERVICE, "--no-pager")
   fmt.Println(out)

  case "6":
   fmt.Println("Tor IP:", getTorIP())

  case "7":
   rotateIP()

  case "8":
   fmt.Print("Country code (DE, NL, FR...): ")
   c, _ := reader.ReadString('\n')
   setExitCountry(strings.TrimSpace(c))

  case "9":
   fmt.Print("Rotate every N minutes: ")
   m, _ := reader.ReadString('\n')
   setAutoRotate(strings.TrimSpace(m))

  case "0":
   logInfo("Exit requested")
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
 requireRoot()
 menu()
}
