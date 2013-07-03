package main

import "encoding/json"
import "flag"
import "github.com/mrjones/oauth"
import "io"
import "io/ioutil"
import "net/url"
import "os"
import "os/exec"
import "path"
import "path/filepath"
import "strings"

var prefix = ""

func ck(e error) {
  if e != nil {
    panic(e)
  }
}

func main() {
  var verbose bool
  flag.BoolVar(&verbose, "v", false, "verbosity")
  flag.Parse()
  args := flag.Args()

  var err error
  if len(args) < 1 {
    print("usage: ")
    print(os.Args[0])
    println(" <path to sync to>")
    return
  }

  prefix, err = filepath.Abs(args[0])
  ck(err)

  sp := oauth.ServiceProvider{
    RequestTokenUrl:   "https://api.dropbox.com/1/oauth/request_token",
    AuthorizeTokenUrl: "https://www.dropbox.com/1/oauth/authorize",
    AccessTokenUrl:    "https://api.dropbox.com/1/oauth/access_token" }

  c := oauth.NewConsumer(AppKey, AppSecret, sp)
  token := token(c)

  type Delta struct {
    Cursor  string  `json:"cursor"`
    Reset   bool    `json:"reset"`
    HasMore bool    `json:"has_more"`
    Entries [][]interface{}
  }
  delta := Delta{Cursor: loadCursor(), HasMore: true}
  params := make(map[string]string)
  updated := false

  for delta.HasMore {
    params["cursor"] = delta.Cursor
    res, err := c.Post("https://api.dropbox.com/1/delta", params, token)
    ck(err)
    str, err := ioutil.ReadAll(res.Body)
    ck(err)
    res.Body.Close()
    json.Unmarshal([]byte(str), &delta)
    for _, arr := range delta.Entries {
      updated = true
      rel := arr[0].(string)
      meta, ok := arr[1].(map[string]interface{})
      if !ok {
        os.RemoveAll(prefix + rel)
      } else {
        if meta["is_dir"].(bool) {
          os.Mkdir(prefix + rel, 0755)
        } else {
          p := "https://api-content.dropbox.com/1/files/sandbox"
          p += strings.Replace(url.QueryEscape(rel), "%2F", "/", -1)
          r, err := c.Get(p, nil, token)
          ck(err)
          os.MkdirAll(path.Dir(prefix + meta["path"].(string)), 0755)
          f, err := os.Create(prefix + meta["path"].(string))
          ck(err)
          io.Copy(f, r.Body)
          f.Close()
          r.Body.Close()
        }
      }
      if verbose {
        println("updated ", rel)
      }
    }
    saveCursor(delta.Cursor)
  }
  err = os.Chdir(prefix)
  ck(err)

  if !updated { return }
  exec.Command("/bin/sh", ".after-sync").Run()
}

func saveCursor(cursor string) {
  f, err := os.Create(prefix + "/.cursor")
  ck(err)
  _, err = f.Write([]byte(cursor))
  ck(err)
  f.Close()
}

func loadCursor() string {
  f, err := os.Open(prefix + "/.cursor")
  if err != nil { return "" }
  defer f.Close()
  cursor, err := ioutil.ReadAll(f)
  if err != nil { return "" }
  return string(cursor)
}

func saved() *oauth.AccessToken {
  token := oauth.AccessToken{}
  file, err := os.Open(prefix + "/.credentials")
  if err != nil { return nil }
  defer file.Close()
  bytes, err := ioutil.ReadAll(file)
  if err != nil { return nil }
  parts := strings.Split(string(bytes), "\n")
  token.Token = parts[0]
  token.Secret = parts[1]
  return &token
}

func save(t *oauth.AccessToken) {
  err := os.MkdirAll(prefix, 0755)
  ck(err)
  file, err := os.Create(prefix + "/.credentials")
  ck(err)
  _, err = file.Write([]byte(t.Token))
  ck(err)
  _, err = file.Write([]byte{'\n'})
  ck(err)
  _, err = file.Write([]byte(t.Secret))
  ck(err)
  file.Close()
}

func token(c *oauth.Consumer) *oauth.AccessToken {
  atoken := saved()
  if atoken != nil {
    return atoken
  }
  token, login, err := c.GetRequestTokenAndUrl("")
  ck(err)
  println(login)
  print("Hit enter when authorized...")
  _, err = os.Stdin.Read([]byte{0})
  ck(err)
  atoken, err = c.AuthorizeToken(token, "")
  ck(err)
  save(atoken)
  return atoken
}
