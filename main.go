package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/freightcms/logging"
	"github.com/freightcms/organizations/db"
	"github.com/freightcms/organizations/db/mongodb"
	"github.com/freightcms/organizations/web"
	dotenv "github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	port           int
	host           string
	dbName         string
	collectionName string
	allowedHosts   string
	logger         = logging.New(os.Stdout, logging.LogLevels())
	client         *mongo.Client
)

// addMongoDbMiddleware adds the CarrierResourceManager to the echo context so that it can be
// be recovered from the db.DbContext object
func addMongoDbMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return echo.HandlerFunc(func(c echo.Context) error {
		if client == nil {
			return next(c)
		}
		session, err := client.StartSession()
		if err != nil {
			return err
		}
		requestContext := c.Request().Context()
		defer session.EndSession(requestContext)

		sessionContext := mongo.NewSessionContext(requestContext, session)
		dbContext := db.DbContext{
			Context:                     requestContext,
			OrganizationResourceManager: mongodb.NewOrganizationManager(dbName, collectionName, sessionContext),
		}
		wrappedContext := web.AppContext{
			Context:   c,
			DbContext: dbContext,
		}
		return next(wrappedContext)
	})
}

func loggingMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return echo.HandlerFunc(func(c echo.Context) error {
		req := c.Request()

		headers := make(map[string][]string)
		for k, v := range req.Header {
			if k == "authorization" {
				continue // don't include this header for security reasons
			}
			headers[k] = v
		}
		logObj := struct {
			Body    string              `json:"body"`
			Headers map[string][]string `json:"headers"`
			Method  string              `json:"method"`
			Url     string              `json:"url"`
		}{
			Headers: headers,
			Method:  req.Method,
			Url:     req.URL.String(),
		}

		logObjJson, _ := json.Marshal(logObj)
		if len(logObjJson) != 0 {
			logger.Debug(string(logObjJson))
		}
		err := next(c)
		if err != nil {
			jsonError, _ := json.Marshal(err)
			if len(jsonError) != 0 {
				logger.Error(string(jsonError))
			}
		}
		return err
	})
}

// getAllowedOrigins parses the`allowedHosts` global variable in a sane list of strings by trimming the strings and spliting on the "," character
func getAllowedOrigins() []string {
	s := strings.Split(allowedHosts, ",")
	origins := make([]string, len(s))

	for _, v := range s {
		origins = append(origins, strings.Trim(v, " "))
	}

	return origins
}

func main() {
	var err error

	flag.IntVar(&port, "p", 8080, "Port to run application on")
	flag.StringVar(&host, "h", "0.0.0.0", "Host address to run application on")
	flag.StringVar(&dbName, "database", "freightcms", "Name of the database to use when connecting. Defaults to freightcms")
	flag.StringVar(&collectionName, "collection", "people", "Name of the collection in mongodb to use when connecting. Defaults to 'people'")
	flag.StringVar(&allowedHosts, "allowedHosts", "localhost:8080", "Comma separated list of hostname that are allowed to communicate with service")
	flag.Parse()

	ctx := context.Background()

	logger.Debug("Starting application...")

	if err = dotenv.Load(".env"); err != nil {
		logger.Warning("Could not find \".env\" file")
	}
	if dbName == "" {
		dbName = os.Getenv("DATABASE_NAME")
		if dbName == "" {
			log.Fatal("Could not get database name from environment or cli option '--database=...'")
		}
	}

	if collectionName == "" {
		dbName = os.Getenv("COLLECTION_NAME")
		if dbName == "" {
			log.Fatal("Could not get collection name from environment or cli option '--collection=...'")
		}
	}
	if len(allowedHosts) == 0 {
		logger.Warning("Starting application with no Allowed Hosts")
	}

	serverAPI := options.ServerAPI(options.ServerAPIVersion1)
	opts := options.Client().ApplyURI(os.Getenv("MONGO_SERVER")).SetServerAPIOptions(serverAPI)
	client, err = mongo.Connect(ctx, opts)
	if err != nil {
		log.Fatal(err)
		return
	}

	defer client.Disconnect(ctx)
	logger.Debug("Pinging server...")
	if err = client.Ping(ctx, nil); err != nil {
		log.Fatal(err)
		return
	}

	logger.Debug("Done")
	logger.Debug("Setting up handlers and routes")

	server := echo.New()
	server.Use(middleware.Secure())
	server.Use(middleware.CSRF())
	server.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: getAllowedOrigins(),
		AllowMethods: []string{
			http.MethodGet,
			http.MethodDelete,
			http.MethodOptions,
			http.MethodPost,
			http.MethodPut,
		},
	}))

	server.Use(loggingMiddleware)
	server.Use(addMongoDbMiddleware)

	web.Register(server)

	logger.Debug("Done")
	hostname := fmt.Sprintf("%v:%d", host, port)
	logger.Debug("Start server at %s\n", hostname)

	http.ListenAndServe(hostname, server)
}
