package main

import (
	"flag"
	"io/ioutil"
	"os"
	"path/filepath"

	"jaeger-postgresql/pgstore"

	"github.com/hashicorp/go-hclog"
	"github.com/jaegertracing/jaeger/plugin/storage/grpc"
	"github.com/jaegertracing/jaeger/plugin/storage/grpc/shared"
	"gopkg.in/yaml.v2"
)

func main() {
	logger := hclog.New(&hclog.LoggerOptions{
		Name:  "jaeger-postgresql",
		Level: hclog.Debug, // Jaeger only captures >= Warn, so don't bother logging below Warn
	})

	var store shared.StoragePlugin
	var closeStore func() error
	var err error

	conf := pgstore.Configuration{}

	var configPath string
	flag.StringVar(&configPath, "config", "", "The absolute path to the plugin's configuration file")
	flag.Parse()

	cfgFile, err := ioutil.ReadFile(filepath.Clean(configPath))
	if err != nil {
		logger.Error("Could not read config file", "config", configPath, "error", err)
		os.Exit(1)
	}

	err = yaml.Unmarshal(cfgFile, &conf)
	if err != nil {
		logger.Error("Could not parse config file", "error", err)
	}

	store, closeStore, err = pgstore.NewStore(&conf, logger)

	grpc.Serve(store)

	if err = closeStore(); err != nil {
		logger.Error("failed to close store", "error", err)
		os.Exit(1)
	}
}
