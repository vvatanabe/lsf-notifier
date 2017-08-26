package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"

	"flag"
	"os"

	"github.com/PuerkitoBio/goquery"
	"github.com/garyburd/redigo/redis"
	"github.com/nulab/go-typetalk/typetalk"
	"path"
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
	var configFilePath string
	flag.StringVar(&configFilePath, "config", defaultConfFilePath, "config file path")
	flag.StringVar(&configFilePath, "c", defaultConfFilePath, "config file path")
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
			fmt.Print("Scraping failed. House ID: " + houseId)
		}

		houseName := doc.Find("table.jyu_table tr td span").First().Text()
		hitNumBox := doc.Find("#hitnum_box")

		currentHitNumBoxText := hitNumBox.Text()

		hitNumBoxTextKey := "HitNumBoxText-" + houseId
		beforeHitNumBoxText, err := redis.String(c.Do("GET", hitNumBoxTextKey))
		if err != nil {
			fmt.Println("Before house data does not exist. House name: " + houseName)
		}

		if currentHitNumBoxText == beforeHitNumBoxText {
			fmt.Println("It's the same as the before state. House name: " + houseName)
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
		fmt.Println(message)
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
