// genkey 生成 gameserver 会话密钥 RSA 密钥对：
//
//	go run ./cmd/genkey -out deploy/data/login_key.pem
//
// 输出 PKCS#1 私钥 PEM（HS_GAME_RSA_KEY 指向它）与 openssh 公钥行（嵌进启动期热修脚本，
// 替换客户端 network_mgr.gate_client_config['loginkeycontent']，见 out/gate-protocol-spec.md）。
package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"flag"
	"fmt"
	"log"
	"os"

	"golang.org/x/crypto/ssh"
)

func main() {
	out := flag.String("out", "deploy/data/login_key.pem", "私钥 PEM 输出路径")
	bits := flag.Int("bits", 2048, "RSA 位数")
	flag.Parse()
	key, err := rsa.GenerateKey(rand.Reader, *bits)
	if err != nil {
		log.Fatal(err)
	}
	body := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	if err := os.WriteFile(*out, body, 0600); err != nil {
		log.Fatal(err)
	}
	pub, err := ssh.NewPublicKey(&key.PublicKey)
	if err != nil {
		log.Fatal(err)
	}
	authorized := string(ssh.MarshalAuthorizedKey(pub)) // 单行 "ssh-rsa AAAA...\n"
	fmt.Println("私钥已写入", *out)
	fmt.Println("openssh 公钥（嵌入启动期热修替换 loginkeycontent）：")
	fmt.Print(authorized)
	fmt.Println("\n热修脚本示例（deploy/data/hotfix.json 的 startup_scripts[\"1.0.125\"]）：")
	fmt.Printf("import client.network_mgr as nm\nnm.gate_client_config['loginkeycontent'] = %q\n", authorized[:len(authorized)-1])
}
