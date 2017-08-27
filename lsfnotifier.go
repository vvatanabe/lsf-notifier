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

	"strings"

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

const (
	documentUrl        = "https://lsf.jp/rent/nam_1.php"
	houseDetailBaseUrl = "https://lsf.jp/rent/bui_1.php"
)

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
		logFilePath    string
	)
	flag.StringVar(&configFilePath, "config", defaultConfFilePath, "config file path")
	flag.StringVar(&configFilePath, "c", defaultConfFilePath, "config file path")
	flag.StringVar(&logFilePath, "log", "", "log file path")
	flag.Parse()

	// ---------------------------------------
	// parse config
	// ---------------------------------------
	notifierConfig, err := parseConfig(configFilePath)
	if err != nil {
		fmt.Println("parse config error: " + err.Error())
		return
	}

	// ---------------------------------------
	// setup typetalk
	// ---------------------------------------
	client := typetalk.NewClient(nil).SetTypetalkToken(notifierConfig.TypetalkToken)

	// ---------------------------------------
	// setup log
	// ---------------------------------------
	if 0 < len(logFilePath) {
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
		Flag: log.Ldate | log.Ltime | log.Lshortfile,
	})
	colog.Register()

	// ---------------------------------------
	// setup redis
	// ---------------------------------------
	c, err := redis.Dial(notifierConfig.RedisNetWork, notifierConfig.RedisPort)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer c.Close()

	// ---------------------------------------
	// main
	// ---------------------------------------
	doc, err := goquery.NewDocument(documentUrl)
	if err != nil {
		log.Printf("error: Scraping failed. Document URL: %s", documentUrl)
	}

	for _, houseId := range notifierConfig.HouseIds {
		selector := ".nam_table tbody tr td ul li a[href='bui_1.php?dn=" + houseId + "']"
		houseInfo := doc.Find(selector).First().Text()

		startBracketsIndex := strings.Index(houseInfo, "【")
		endBracketsIndex := strings.Index(houseInfo, "】")

		houseName := houseInfo[0:startBracketsIndex]
		currentHouseCount := houseInfo[startBracketsIndex+3 : endBracketsIndex]

		houseCountKey := "HOUSE_" + houseId + "_COUNT"
		beforeHouseCount, err := redis.String(c.Do("GET", houseCountKey))
		if err != nil {
			log.Printf("info: Before house count data does not exist. House name: %s", houseName)
		}

		if currentHouseCount == beforeHouseCount {
			log.Printf(
				"info: It's the same as the before state. current: %s, before: %s, House name: %s",
				currentHouseCount,
				beforeHouseCount,
				houseName,
			)
			continue
		}

		c.Do("SET", houseCountKey, currentHouseCount)

		houseDetailUrl := houseDetailBaseUrl + "?dn=" + houseId

		message := "[" + houseInfo + "](" + houseDetailUrl + ")"
		client.Messages.PostMessage(context.Background(), notifierConfig.TypetalkTopicId, message, nil)
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
