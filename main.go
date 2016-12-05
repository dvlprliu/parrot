package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/anthonynsimon/parrot/api"
	"github.com/anthonynsimon/parrot/auth"
	"github.com/anthonynsimon/parrot/datastore"
	"github.com/anthonynsimon/parrot/logger"
	"github.com/joho/godotenv"
	"github.com/pressly/chi"
	"github.com/pressly/chi/middleware"
)

func init() {
	// Config log
	logrus.SetOutput(os.Stdout)
	logrus.SetFormatter(&logrus.TextFormatter{})
	logrus.SetLevel(logrus.InfoLevel)
}

func main() {
	// init environment variables
	err := godotenv.Load()
	if err != nil {
		logrus.Fatal(err)
	}

	// init and ping datastore
	dbName := os.Getenv("PARROT_DB_NAME")
	dbURL := os.Getenv("PARROT_DB_URL")
	if dbName == "" || dbURL == "" {
		logrus.Fatal("no db set in env")
	}

	ds, err := datastore.NewDatastore(dbName, dbURL)
	if err != nil {
		logrus.Fatal(err)
	}
	defer ds.Close()

	// Ping DB until service is up, block meanwhile
	blockAndRetry(5*time.Second, func() bool {
		if err = ds.Ping(); err != nil {
			logrus.Error(fmt.Sprintf("failed to ping datastore.\nerr: %s", err))
			return false
		}
		return true
	})

	// init routers and middleware
	// CORS, Rate-limiting, etc... is handled in nginx
	// Here we only care about application level middleware
	mainRouter := chi.NewRouter()
	mainRouter.Use(
		middleware.RequestID,
		middleware.RealIP,
		logger.Request,
		middleware.StripSlashes,
	)

	apiKey := os.Getenv("PARROT_API_SIGNING_KEY")
	if apiKey == "" {
		logrus.Fatal("no api key set in env")
	}
	domain := os.Getenv("PARROT_DOMAIN")
	if apiKey == "" {
		logrus.Fatal("no domain set in env")
	}

	ap := auth.Provider{
		Name:       string([]byte(domain)),
		SigningKey: []byte(apiKey)}

	apiRouter := api.NewRouter(ds, ap)
	mainRouter.Mount("/api", apiRouter)

	// config server
	addr := ":8080"

	// init server
	s := &http.Server{
		Addr:           addr,
		Handler:        mainRouter,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	logrus.Info(fmt.Sprintf("Listening on %s", addr))

	logrus.Fatal(s.ListenAndServe())
}

func blockAndRetry(d time.Duration, fn func() bool) {
	for !fn() {
		fmt.Printf("retrying in %s...\n", d.String())
		time.Sleep(d)
	}
}
