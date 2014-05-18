package main

import (
	"flag"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/codegangsta/martini"
	"github.com/gorilla/websocket"
	"github.com/martini-contrib/binding"
	"github.com/martini-contrib/render"

	"net/http"
	_ "net/http/pprof"
)

func init() {

}

func main() {
	web := flag.String("web", "localhost:3000", "set the IP/port of the web server")
	flag.Parse()

	if *web != "" {
		webs := strings.Split(*web, ":")
		os.Setenv("HOST", webs[0])
		os.Setenv("PORT", webs[1])
	}

	cmds := make(chan exec.Cmd)
	results := make(chan Result)

	// sends command to be executed from martini to client wrangler
	go func() {
		for {
			select {
			// run commands given
			case cmd := <-cmds:
				output, err := cmd.CombinedOutput()
				if err != nil {
					results <- Result{
						Output: string(output),
						Error:  err.Error(),
					}
				} else {
					results <- Result{
						Output: string(output),
						Error:  "",
					}
				}
			}
		}
	}()

	// for debug
	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()

	// some kind of interface for user to give commands
	m := martini.Classic()
	m.Use(render.Renderer())
	m.Get("/", func(r render.Render) {
		select {
		case data := <-results:
			r.HTML(200, "index", data)
		default:
			r.HTML(200, "index", nil)
		}
	})
	m.Post("/cmd", binding.Json(CmdPayload{}), func(cp CmdPayload, r render.Render) {
		if cp.Cmd == "" {
			r.Redirect("/")
			return
		}
		if strings.Contains(cp.Cmd, "&&") || strings.Contains(cp.Cmd, "||") {
			results <- Result{
				Output: "",
				Error:  "Ands n Ors ain't supported",
			}
			return
		}
		pipelines := strings.Split(cp.Cmd, "|")
		l := len(pipelines)
		log.Printf("Pipeline length: %v", l)
		var lastCmdStdout io.ReadCloser
		var err error

		log.Println("Running command", cp.Cmd)
		for i, c := range pipelines {
			parts := strings.Split(strings.TrimSpace(c), " ")

			var cmd *exec.Cmd
			if len(parts) > 1 {
				cmd = exec.Command(parts[0], parts[1:]...)
			} else {
				cmd = exec.Command(parts[0])
			}
			if l > 1 { // pipeline!
				if i == 0 { // first
					lastCmdStdout, err = cmd.StdoutPipe()
					if err != nil {
						log.Println(err)
						return
					}
					cmd.Start()
				} else if i < l-1 { // middle
					cmd.Stdin = lastCmdStdout
					lastCmdStdout, err = cmd.StdoutPipe()
					if err != nil {
						log.Println(err)
						return
					}
					cmd.Start()
				} else { // last
					cmd.Stdin = lastCmdStdout
					cmds <- *cmd
				}
			} else {
				cmds <- *cmd
			}
		}

		r.Redirect("/")
	})
	m.Get("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Upgrade(w, r, nil, 1024, 1024)
		if _, ok := err.(websocket.HandshakeError); ok {
			http.Error(w, "Not a websocket handshake", 400)
			return
		} else if err != nil {
			log.Println(err)
			return
		}
		defer conn.Close()
		log.Println("Succesfully upgraded connection")

		for {
			err = conn.WriteJSON(<-results)
			if err != nil {
				log.Println(err)
				return
			}
		}
	})
	m.Run()
}

type CmdPayload struct {
	Cmd string `json:"cmd"`
}

type Result struct {
	Output string `json:"output"`
	Error  string `json:"error"`
}
