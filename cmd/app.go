package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"netflix-node-select/config"
	"os"
	"strings"

	"github.com/Dreamacro/clash/constant"
	"github.com/Dreamacro/clash/hub/executor"
	http2 "github.com/Dreamacro/clash/listener/http"
	"github.com/sjlleo/netflix-verify/verify"
)

var proxy constant.Proxy

func Run(args []string) {
	config.LoadConfig(args[1])

	url := config.ReadConfig("clash.subscription_url")
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=UTF-8")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	clashConfig, err := executor.ParseWithBytes(body)
	if err != nil {
		log.Fatal(err)
	}
	nodes := clashConfig.Proxies
	in := make(chan constant.ConnContext, len(nodes))
	defer close(in)

	l, err := http2.New("127.0.0.1:10000", in)
	if err != nil {
		panic(err)
	}
	defer l.Close()
	println("listen at:", l.Address())
	go func() {
		for c := range in {
			conn := c
			metadata := conn.Metadata()
			fmt.Printf("request incoming from %s to %s\n", metadata.SourceAddress(), metadata.RemoteAddress())
			go func() {
				remote, err := proxy.DialContext(context.Background(), metadata)
				if err != nil {
					fmt.Printf("dial error: %s\n", err.Error())
					return
				}
				relay(remote, conn.Conn())
			}()
		}
	}()
	var ress string
forLoop:
	for node, server := range nodes {
		if server.Type() != constant.Shadowsocks && server.Type() != constant.ShadowsocksR && server.Type() != constant.Snell && server.Type() != constant.Socks5 && server.Type() != constant.Http && server.Type() != constant.Vmess && server.Type() != constant.Trojan {
			continue
		}

		if !strings.Contains(server.Name(), "HK") && !strings.Contains(server.Name(), "TW") && !strings.Contains(server.Name(), "SG") {
			continue
		}

		proxy = server

		//Netflix检测
		r := verify.NewVerify(verify.Config{
			Proxy: "http://127.0.0.1:10000",
		})
		switch r.Res[1].StatusCode {
		case 2:
			if r.Res[1].CountryName == "台湾" || r.Res[1].CountryName == "香港" || r.Res[1].CountryName == "新加坡" {
				ress = node
				break forLoop
			}
		default:
		}
	}
	var jsonStr = []byte(`{"name":"` + ress + `"}`)
	posturl := config.ReadConfig("clash.node_url")
	beartoken := config.ReadConfig(("clash.bearer"))
	req2, err := http.NewRequest("PUT", posturl, bytes.NewBuffer(jsonStr))
	req2.Header.Add("Authorization", beartoken)
	req2.Header.Add("Content-Type", "application/json")
	if err != nil {
		log.Fatal(err)
	}
	_, err = http.DefaultClient.Do(req2)
	if err != nil {
		log.Fatal(err)
	}
	os.Exit(0)
}

func relay(l, r net.Conn) {
	go io.Copy(l, r)
	io.Copy(r, l)
}
