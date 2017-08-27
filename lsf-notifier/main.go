package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"

	"flag"
	"os"

	"log"
	"path"

	"github.com/PuerkitoBio/goquery"
	"github.com/comail/colog"
	"github.com/garyburd/redigo/redis"
	"github.com/nulab/go-typetalk/typetalk"
)

type Config struct {
	HouseIds        []string `json:"house_ids"`
	TypetalkTopicId int      `json:"typetalk_topic_id"`
	TypetalkToken   string   `json:"typetalk_token"`
	RedisNetWork    string   `json:"redis_network"`
	RedisPort       string   `json:"redis_port"`
}

func main() {

	// ---------------------------------------
	// parse command line args
	// ---------------------------------------
	executablePath, err := os.Executable()
	if err != nil {
		fmt.Println("os.Executable error: " + err.Error())
		return
	}

	defaultConfFilePath := path.Dir(executablePath) + "/conf.json"
	var (
		configFilePath string
		logFilePath string
	)
	flag.StringVar(&configFilePath, "config", defaultConfFilePath, "config file path")
	flag.StringVar(&configFilePath, "c", defaultConfFilePath, "config file path")
	flag.StringVar(&logFilePath, "log", "", "log file path")
	flag.Parse()

	// ---------------------------------------
	// parse config
	// ---------------------------------------
	config, err := parseConfig(configFilePath)
	if err != nil {
		fmt.Println("parse config error: " + err.Error())
		return
	}

	// ---------------------------------------
	// setup log
	// ---------------------------------------
	if (0 < len(logFilePath)) {
		file, err := os.OpenFile(logFilePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0777)
		if err != nil {
			fmt.Println("open log file error: " + err.Error())
			return
		}
		colog.SetOutput(file)
	}
	colog.SetDefaultLevel(colog.LDebug)
	colog.SetMinLevel(colog.LTrace)
	colog.SetFormatter(&colog.StdFormatter{
		Flag:   log.Ldate | log.Ltime | log.Lshortfile,
	})
	colog.Register()

	// ---------------------------------------
	// setup redis
	// ---------------------------------------
	c, err := redis.Dial(config.RedisNetWork, config.RedisPort)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer c.Close()

	// ---------------------------------------
	// setup typetalk
	// ---------------------------------------
	client := typetalk.NewClient(nil).SetTypetalkToken(config.TypetalkToken)

	// ---------------------------------------
	// main
	// ---------------------------------------
	for _, houseId := range config.HouseIds {

		houseUrl := "https://lsf.jp/rent/bui_1.php?dn=" + houseId
		doc, err := goquery.NewDocument(houseUrl)
		if err != nil {
			log.Printf("error: Scraping failed. House ID: %s", houseId)
		}

		houseName := doc.Find("table.jyu_table tr td span").First().Text()
		hitNumBox := doc.Find("#hitnum_box")

		currentHitNumBoxText := hitNumBox.Text()

		hitNumBoxTextKey := "HitNumBoxText-" + houseId
		beforeHitNumBoxText, err := redis.String(c.Do("GET", hitNumBoxTextKey))
		if err != nil {
			log.Printf("info: Before house data does not exist. House name: %s", houseName)
		}

		if currentHitNumBoxText == beforeHitNumBoxText {
			log.Printf("info: It's the same as the before state. House name: %s", houseName)
			continue
		}

		c.Do("SET", hitNumBoxTextKey, currentHitNumBoxText)

		message := "House name: " + houseName + "\n"
		hitNumBox.Find("span").Each(func(i int, s *goquery.Selection) {
			if i == 0 {
				message += "Repaired house: " + s.Text() + "\n"
			} else if i == 1 {
				message += "General houes: " + s.Text() + "\n"
			}
		})
		message += "URL: " + houseUrl + "\n"
		ctx := context.Background()
		client.Messages.PostMessage(ctx, config.TypetalkTopicId, message, nil)
		log.Printf("info: %s", message)
	}

}

func parseConfig(path string) (Config, error) {
	var c Config
	jsonString, err := ioutil.ReadFile(path)
	if err != nil {
		return c, err
	}
	err = json.Unmarshal(jsonString, &c)
	if err != nil {
		return c, err
	}
	return c, nil
}
