package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
)

func proxyJSON(c *fiber.Ctx, method, url string) error {

	req, err := http.NewRequest(method, url, bytes.NewReader(c.Body()))
	if err != nil {
		return c.Status(500).SendString(err.Error())
	}

	// Forward headers
	c.Request().Header.VisitAll(func(key, value []byte) {
		req.Header.Set(string(key), string(value))
	})

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return c.Status(502).SendString(err.Error())
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	c.Set("Content-Type", resp.Header.Get("Content-Type"))

	return c.Status(resp.StatusCode).Send(body)
}

func runLauncher(port int, controlPort int, address string, controlAddress string) {

	log.Println("Launcher started")

	var serverArgs []string

	args := os.Args[1:]

	for i := 0; i < len(args); i++ {

		arg := args[i]

		// Skip launcher flags AND their values
		if arg == "-port" {
			i++ // skip value
			continue
		}

		if arg == "-server" {
			continue
		}

		serverArgs = append(serverArgs, arg)
	}

	// Add server mode flag
	serverArgs = append([]string{
		"server",
		"-control-port", strconv.Itoa(controlPort),
	}, serverArgs...)

	cmd := exec.Command(os.Args[0], serverArgs...)

	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	cmd.Stdin = os.Stdin

	if err := cmd.Start(); err != nil {
		log.Fatal(err)
	}

	log.Println("Server started with PID", cmd.Process.Pid)

	go io.Copy(os.Stdout, stdout)
	go io.Copy(os.Stderr, stderr)

	stopDashboard := startSupervisorDashboard(port, cmd, controlPort, address, controlAddress)

	go func() {
		err := cmd.Wait()
		log.Println("Server exited:", err)
	}()

	select {}

	_ = stopDashboard
}

func startSupervisorDashboard(port int, cmd *exec.Cmd, controlPort int, address string, controlAddress string) func() {

	app := fiber.New()

	app.Static("/", "./web")

	app.Get("/api/server/status", func(c *fiber.Ctx) error {
		running := cmd.ProcessState == nil

		return c.JSON(fiber.Map{
			"running": running,
			"pid":     cmd.Process.Pid,
		})
	})

	app.Post("/api/server/stop", func(c *fiber.Ctx) error {
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		return c.SendStatus(204)
	})

	app.Post("/api/server/start", func(c *fiber.Ctx) error {

		if cmd.ProcessState == nil {
			return c.SendString("Already running")
		}

		newCmd := exec.Command(os.Args[0], "server")
		newCmd.Stdout = os.Stdout
		newCmd.Stderr = os.Stderr

		if err := newCmd.Start(); err != nil {
			return c.Status(500).SendString(err.Error())
		}

		cmd = newCmd
		return c.SendStatus(204)
	})

	base := fmt.Sprintf("http://%s:%d", controlAddress, controlPort)

	app.Get("/api/invite", func(c *fiber.Ctx) error {
		return proxyJSON(c, "GET", base+"/status")
	})

	app.Get("/api/stats", func(c *fiber.Ctx) error {
		return proxyJSON(c, "GET", base+"/status")
	})

	app.Post("/api/invite", func(c *fiber.Ctx) error {
		return proxyJSON(c, "POST", base+"/invite")
	})

	app.Get("/api/tracks", func(c *fiber.Ctx) error {
		return proxyJSON(c, "GET", base+"/status")
	})

	app.Post("/api/tracks", func(c *fiber.Ctx) error {
		return proxyJSON(c, "POST", base+"/track")
	})

	app.Post("/api/kick", func(c *fiber.Ctx) error {
		return proxyJSON(c, "POST", base+"/kick")
	})
	app.Post("/api/session/end", func(c *fiber.Ctx) error {
		return proxyJSON(c, "POST", base+"/session/end")
	})
	app.Post("/api/session/start", func(c *fiber.Ctx) error {
		return proxyJSON(c, "POST", base+"/session/start")
	})
	app.Post("/api/session/set", func(c *fiber.Ctx) error {
		return proxyJSON(c, "POST", base+"/session/set")
	})

	app.Post("/api/autorotate/start", func(c *fiber.Ctx) error {
		return proxyJSON(c, "POST", base+"/autorotate/start")
	})
	app.Post("/api/autorotate/stop", func(c *fiber.Ctx) error {
		return proxyJSON(c, "POST", base+"/autorotate/stop")
	})
	app.Post("/api/autorotate/skip", func(c *fiber.Ctx) error {
		return proxyJSON(c, "POST", base+"/autorotate/skip")
	})

	app.Post("/api/reloadTracks", func(c *fiber.Ctx) error {
		return proxyJSON(c, "POST", base+"/reloadTracks")
	})

	app.Get("/api/players", func(c *fiber.Ctx) error {
		return proxyJSON(c, "GET", base+"/players")
	})

	addr := fmt.Sprintf("%s:%d", address, port)

	go func() {
		log.Println("Dashboard running on http://" + addr)
		if err := app.Listen(addr); err != nil {
			log.Println(err)
		}
	}()

	return func() {
		app.ShutdownWithTimeout(5 * time.Second)
	}
}

func main() {

	// Server mode (positional argument)
	if len(os.Args) > 1 && os.Args[1] == "server" {
		runServer()
		return
	}

	launcherFlags := flag.NewFlagSet("launcher", flag.ContinueOnError)

	address := launcherFlags.String("address", "127.0.0.1", "dashboard address")
	controlAddress := launcherFlags.String("control-address", "127.0.0.1", "server control address")
	portFlag := launcherFlags.Int("port", 8080, "dashboard port")
	controlPort := launcherFlags.Int("control-port", 9090, "server control port")

	err := launcherFlags.Parse(os.Args[1:])
	if err != nil {
		log.Fatalln("Failed parsing flags!")
	}
	runLauncher(*portFlag, *controlPort, *address, *controlAddress)

}
