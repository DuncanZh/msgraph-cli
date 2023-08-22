package main

import (
	"encoding/json"
	"fmt"
	"github.com/Joker-Jane/msgraph-cli/api"
	"github.com/abiosoft/ishell/v2"
	"github.com/abiosoft/readline"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	defer func() {
		if err := recover(); err != nil {
			fmt.Println(err)
		}
	}()

	VERSION := "0.1.0"
	PROMPT := "msgraph_cli v" + VERSION + " $ "

	shell := ishell.NewWithConfig(&readline.Config{Prompt: PROMPT})
	shell.SetHomeHistoryPath(".msgraph_shell_history")

	g := api.NewGraphAPI()
	shell.Set("api", g)

	list := []string{"users", "groups", "applications", "servicePrincipals"}
	get := []string{"authentication/methods", "appRoleAssignments"}

	shell.Set("list", list)
	shell.Set("get", get)

	shell.AddCmd(&ishell.Cmd{
		Name: "auth",
		Help: "Usage: auth <credential_file> OR auth <client_id> <client_secret> <tenant_id>",
		CompleterWithPrefix: func(prefix string, args []string) []string {
			if len(args) == 0 {
				return getDirectory(prefix)
			}
			return []string{}
		},
		Func: auth,
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "list",
		Help: "Usage: list <resource> <output_file> [expand]",
		CompleterWithPrefix: func(prefix string, args []string) []string {
			if len(args) == 0 {
				return shell.Get("list").([]string)
			} else if len(args) == 1 {
				return getDirectory(prefix)
			}
			return []string{}
		},
		Func: listResource,
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "get",
		Help: "Usage: get <source> <resource> <users_file> <output_file> [expand]",
		CompleterWithPrefix: func(prefix string, args []string) []string {
			if len(args) == 0 {
				return []string{"users", "groups", "servicePrincipals"}
			} else if len(args) == 1 {
				return shell.Get("get").([]string)
			} else if len(args) < 4 {
				return getDirectory(prefix)
			}
			return []string{}
		},
		Func: getResource,
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "test",
		Help: "Usage: Develop use only",
		Func: test,
	})

	if len(os.Args) > 1 {
		err := shell.Process(os.Args[1:]...)
		if err != nil {
			fmt.Println("Error: " + err.Error())
		}
	} else {
		shell.Println("Interactive Shell: Please use 'auth' command to authenticate the API")
		shell.Run()
		shell.Close()
	}
}

func auth(c *ishell.Context) {
	clientId := ""
	clientSecret := ""
	tenantId := ""

	if len(c.Args) == 3 {
		clientId = c.Args[0]
		clientSecret = c.Args[1]
		tenantId = c.Args[2]
	} else if len(c.Args) == 1 {
		credentialFile, err := os.ReadFile(c.Args[0])
		if err != nil {
			fmt.Println("Error: Failed to read the input file")
			return
		}

		var credentialMap map[string]string
		err = json.Unmarshal(credentialFile, &credentialMap)
		if err != nil {
			fmt.Println("Error: Failed to unmarshal JSON file")
			return
		}

		clientId = credentialMap["clientId"]
		clientSecret = credentialMap["clientSecret"]
		tenantId = credentialMap["tenantId"]
	} else {
		fmt.Println(c.Cmd.Help)
		return
	}

	g := c.Get("api").(*api.GraphAPI)

	err := g.InitializeGraphForUserAuth(clientId, clientSecret, tenantId)
	if err != nil {
		fmt.Println("Error: " + err.Error())
		return
	}

	fmt.Println("Success: Graph API authenticated")
}

func listResource(c *ishell.Context) {
	start := time.Now()

	g := c.Get("api").(*api.GraphAPI)
	if !g.IsInitiated() {
		fmt.Println("Error: Use 'auth' command to authenticate the API before use")
		return
	}

	var expand []string
	if len(c.Args) == 3 {
		expand = []string{c.Args[2]}
	} else if len(c.Args) != 2 {
		fmt.Println(c.Cmd.Help)
		return
	}

	resource := c.Args[0]
	outputFile := c.Args[1]

	result := g.ListResource(resource, expand)
	if result == nil {
		fmt.Printf("Failed: Unable to process the input in %.2f seconds\n", time.Since(start).Seconds())
		return
	}

	if dumpFile(result, outputFile, true) {
		fmt.Printf("Success: Processed %v entries in %.2f seconds\n", len(result), time.Since(start).Seconds())
	}
}

func getResource(c *ishell.Context) {
	start := time.Now()

	g := c.Get("api").(*api.GraphAPI)
	if !g.IsInitiated() {
		fmt.Println("Error: Use 'auth' command to authenticate the API before use")
		return
	}

	var expand []string
	if len(c.Args) == 5 {
		expand = []string{c.Args[4]}
	} else if len(c.Args) != 4 {
		fmt.Println(c.Cmd.Help)
		return
	}

	source := c.Args[0]
	resource := c.Args[1]
	inputFile := c.Args[2]
	outputFile := c.Args[3]

	ids := getIds(inputFile)
	if ids == nil {
		return
	}

	result := g.GetResourceByIdsConcurrent(c, source, resource, ids, expand)
	if result == nil {
		fmt.Printf("Failed: Unable to process the input in %.2f seconds\n", time.Since(start).Seconds())
		return
	}

	if dumpFile(result, outputFile, true) {
		fmt.Printf("Success: Processed %v entries in %.2f seconds\n", len(ids), time.Since(start).Seconds())
	}
}

func test(c *ishell.Context) {
	g := c.Get("api").(*api.GraphAPI)
	if !g.IsInitiated() {
		fmt.Println("Error: Use 'auth' command to authenticate the API before use")
		return
	}

	g.Test()
}

func getDirectory(prefix string) []string {
	path := "./" + prefix
	if f, err := os.Stat(path); err == nil {
		if !f.IsDir() {
			return []string{prefix}
		}
	} else {
		path = path[:strings.LastIndex(path, "/")] + "/"
	}

	entries, _ := os.ReadDir(path)

	var es []string
	for _, e := range entries {
		if path == "./" {
			es = append(es, e.Name())
		} else {
			es = append(es, filepath.Dir(path[2:]+"/")+"/"+e.Name())
		}
	}
	return es
}

func getIds(file string) []string {
	input, err := os.ReadFile(file)
	if err != nil {
		fmt.Println("Error: Failed to read the input file")
		return nil
	}

	var JSON []map[string]interface{}
	err = json.Unmarshal(input, &JSON)
	if err != nil {
		fmt.Println("Error: Failed to parse input JSON")
		return nil
	}

	var result []string
	for _, v := range JSON {
		result = append(result, v["id"].(string))
	}
	return result
}

func dumpFile(result interface{}, file string, pretty bool) bool {
	j, err := json.Marshal(result)
	if pretty {
		j, err = json.MarshalIndent(result, "", " ")
	}

	if err != nil {
		fmt.Println("Error: Failed to encode the output JSON")
		return false
	}

	err = os.MkdirAll(filepath.Dir(file), 0666)
	if err != nil {
		fmt.Println("Error: Failed to create the directory")
		return false
	}

	f, err := os.Create(file)
	if err != nil {
		fmt.Println("Error: Failed to open the output file")
		return false
	}
	defer f.Close()

	_, err = f.Write(j)

	if err != nil {
		fmt.Println("Error: Failed to write to the output file")
		return false
	}

	return true
}
