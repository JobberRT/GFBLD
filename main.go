package main

import (
	"GFBLD/gfbld"
	"fmt"
	nFormatter "github.com/antonfisher/nested-logrus-formatter"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"
)

func init() {
	// init viper
	viper.AutomaticEnv()
	viper.AllowEmptyEnv(true)
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")

	err := viper.ReadInConfig()
	if err != nil {
		logrus.Panic(err)
		os.Exit(-1)
	}

	// init logger
	logrus.SetLevel(logrus.InfoLevel)
	logrus.SetOutput(os.Stdout)
	logrus.SetReportCaller(true)
	logrus.SetFormatter(&nFormatter.Formatter{
		NoColors:        false,
		HideKeys:        false,
		TimestampFormat: time.Stamp,
		CallerFirst:     true,
		CustomCallerFormatter: func(frame *runtime.Frame) string {
			filename := ""
			slash := strings.LastIndex(frame.File, "/")
			if slash >= 0 {
				filename = frame.File[slash+1:]
			}
			return fmt.Sprintf("「%s:%d」", filename, frame.Line)
		},
	})
}

func main() {
	d := gfbld.NewDownloader()
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
		for {
			select {
			case <-c:
				d.Stop()
				os.Exit(0)
			}
		}
	}()
	d.Start()
}
