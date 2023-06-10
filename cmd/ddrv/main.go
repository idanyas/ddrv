package main

import (
    "fmt"
    "log"
    "os"
    "runtime"
    "strings"

    "github.com/alecthomas/kingpin/v2"
    ftpsrvr "github.com/fclairamb/ftpserverlib"
    "github.com/joho/godotenv"

    "github.com/forscht/ddrv/backend/fs"
    "github.com/forscht/ddrv/db"
    "github.com/forscht/ddrv/frontend/ftp"
    "github.com/forscht/ddrv/frontend/http"
    "github.com/forscht/ddrv/pkg/ddrv"
)

// Declare command line flags
var (
    // App initialization and version declaration
    app          = kingpin.New("ddrv", "A utility to use Manager as a file system!").Version(version)
    FTPAddr      = app.Flag("ftpaddr", "Network address for the FTP server to bind to. It defaults to ':2525' meaning it listens on all interfaces.").Envar("FTP_ADDR").Default(":2525").String()
    FTPPortRange = app.Flag("ftppr", "Range of ports to be used for passive FTP connections. The range is provided as a string in the format 'start-end'.").Envar("FTP_PORT_RANGE").Default("").String()
    username     = app.Flag("username", "Username for the ddrv service, used for FTP and HTTP access authentication.").Envar("USERNAME").Default("").String()
    password     = app.Flag("password", "Password for the ddrv service, used for FTP and HTTP access authentication.").Envar("PASSWORD").Default("").String()
    HTTPAddr     = app.Flag("httpaddr", "Network address for the HTTP server to bind to").Envar("HTTP_ADDR").String()
    dbConnStr    = app.Flag("dburl", "Connection string for the Postgres database. The format should be: postgres://user:password@localhost:port/database?sslmode=disable").Envar("DATABASE_URL").Required().String()
    webhooks     = app.Flag("webhooks", "Comma-separated list of Manager webhook URLs used for sending attachment messages.").Envar("DISCORD_WEBHOOKS").Required().String()
    chunkSize    = app.Flag("csize", "The maximum size in bytes of chunks to be sent via Manager webhook. By default, it's set to 24MB (25165824 bytes).").Envar("DISCORD_CHUNK_SIZE").Default("25165824").Int()
)

func main() {
    // Set the maximum number of operating system threads to use.
    runtime.GOMAXPROCS(runtime.NumCPU())

    // Load env file.
    _ = godotenv.Load()

    // Parse command line flags.
    kingpin.MustParse(app.Parse(os.Args[1:]))

    // Make sure chunkSize is below 25MB
    if *chunkSize > 25*1024*1024 || *chunkSize < 0 {
        log.Fatalf("invalid chunkSize %d", chunkSize)
    }
    // Create database connection
    dbConn := db.New(*dbConnStr, false)

    // Create a ddrv manager
    mgr, err := ddrv.NewManager(*chunkSize, strings.Split(*webhooks, ","))
    if err != nil {
        log.Fatalf("failed to open ddrv mgr :%v", err)
    }

    // Create DFS object
    dfs := fs.New(dbConn, mgr)

    var ptr *ftpsrvr.PortRange
    if *FTPPortRange != "" {
        ptr = &ftpsrvr.PortRange{}
        if _, err := fmt.Sscanf(*FTPPortRange, "%d-%d", &ptr.Start, &ptr.End); err != nil {
            log.Fatalf("bad ftp port range %v", err)
        }
    }

    errCh := make(chan error)

    if *FTPAddr != "" {
        go func() {
            // Create and start ftp server
            ftpServer := ftp.New(dfs, *FTPAddr, ptr, *username, *password)
            log.Printf("starting FTP server on : %s", *FTPAddr)
            errCh <- ftpServer.ListenAndServe()
        }()
    }

    if *HTTPAddr != "" {
        go func() {
            httpServer := http.New(*HTTPAddr, dbConn, mgr)
            log.Printf("starting HTTP server on : %s", *HTTPAddr)
            errCh <- httpServer.Serv()
        }()
    }

    log.Fatalf("ddrv error %v", <-errCh)
}
