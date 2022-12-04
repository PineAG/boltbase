package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/boltdb/bolt"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{} // use default options

func registerWebSocket(path string, ws *websocket.Conn) {

	if pathToWebSockets[path] == nil {
		pathToWebSockets[path] = make(map[*websocket.Conn]bool)
	}
	pathToWebSockets[path][ws] = true
}

func recycleWebSocket(path string, ws *websocket.Conn) {
	delete(pathToWebSockets[path], ws)
	if len(pathToWebSockets[path]) == 0 {
		delete(pathToWebSockets, path)
	}
}

func notifyWebSockets(path string, method string) {
	wsSet := pathToWebSockets[path]
	if wsSet == nil {
		return
	}
	for c := range wsSet {
		w, err := c.NextWriter(websocket.TextMessage)
		if err != nil {
			log.Println("WebSocket Notification Error: [NextWriter]", err)
			continue
		}
		_, err2 := w.Write([]byte(method))
		if err2 != nil {
			log.Println("WebSocket Notification Error: [Write]", err)
		}
	}
}

func onWebSocket(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}
	log.Println(path)
	registerWebSocket(path, c)
	defer c.Close()
	defer recycleWebSocket(path, c)
	for {
		_, message, err := c.ReadMessage()
		if err != nil {
			log.Println("read:", err)
			break
		}
		log.Printf("recv: %s", message)
	}
}

var boltBucketName = "data"

func onHttpRequest(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	method := r.Method

	var err error

	switch method {
	case "GET":
		log.Println("GET", path)
		var getData []byte
		err = db.View(func(tx *bolt.Tx) error {
			b := tx.Bucket([]byte(boltBucketName))
			getData = b.Get([]byte(path))
			return nil
		})
		if err != nil {
			log.Println(err)
		}
		if getData == nil {
			w.WriteHeader(http.StatusNotFound)
		} else {
			w.Write(getData)
		}
	case "POST":
		fallthrough
	case "PUT":
		log.Println("SET", path)
		notifyWebSockets(path, "SET")
		log.Println("DEL", path)
		data, err := io.ReadAll(r.Body)
		if err != nil {
			log.Println(err)
			return
		}
		notifyWebSockets(path, "DELETE")
		err = db.Update(func(tx *bolt.Tx) error {
			b := tx.Bucket([]byte(boltBucketName))
			b.Put([]byte(path), data)
			return nil
		})
		if err != nil {
			log.Println(err)
		}

	case "DELETE":
		err = db.Update(func(tx *bolt.Tx) error {
			b := tx.Bucket([]byte(boltBucketName))
			b.Delete([]byte(path))
			return nil
		})
		if err != nil {
			log.Println(err)
		}

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func router(w http.ResponseWriter, r *http.Request) {
	if websocket.IsWebSocketUpgrade(r) {
		onWebSocket(w, r)
	} else {
		onHttpRequest(w, r)
	}
}

var pathToWebSockets = make(map[string]map[*websocket.Conn]bool)

var db *bolt.DB

func connectDB() {
	var err error
	fp := filepath.Join(configDataDir, configDBFileName)
	db, err = bolt.Open(fp, 0600, nil)
	if err != nil {
		log.Fatal(err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(boltBucketName))
		return err
	})
	if err != nil {
		log.Fatal(err)
	}
}

var configDataDir string = "."
var configDBFileName string = "store.db"
var configPort uint64 = 3000

func updateConfig() {
	var err error
	inputDataDir := os.Getenv("DB_ROOT")
	if len(inputDataDir) != 0 {
		configDataDir = inputDataDir
	}
	inputPort := os.Getenv("PORT")
	if len(inputPort) != 0 {
		configPort, err = strconv.ParseUint(inputPort, 10, 64)
		if err != nil {
			log.Fatal(err)
		}
	}
	inputDBFile := os.Getenv("DB_FILE")
	if len(inputDBFile) != 0 {
		configDBFileName = inputDBFile
	}
}

func main() {
	updateConfig()
	connectDB()
	defer db.Close()

	http.HandleFunc("/", router)
	addr := fmt.Sprintf(":%d", configPort)
	log.Fatal(http.ListenAndServe(addr, nil))
}
