package main

import (
	"encoding/json"
	"fmt"
	"github.com/Joker-Jane/msgraph-cli/api"
	"github.com/abiosoft/ishell"
	"github.com/abiosoft/readline"
	"github.com/microsoftgraph/msgraph-sdk-go/users"
	"os"
	"strconv"
	"time"
)

func main() {
	defer func() {
		if err := recover(); err != nil {
			fmt.Println(err)
			println("Sending SIGINT to everyone. (just in case?)")
			//syscall.Kill(syscall.Getpid()*(-1), syscall.SIGINT)
			println("Done")
		}
	}()

	VERSION := "0.1.0"
	PROMPT := "msgraph_cli v" + VERSION + " $ "

	shell := ishell.NewWithConfig(&readline.Config{Prompt: PROMPT})
	shell.SetHomeHistoryPath(".msgraph_shell_history")

	g := api.NewGraphAPI()
	shell.Set("api", g)

	shell.AddCmd(&ishell.Cmd{
		Name: "auth",
		Help: "Usage: auth <credential_file> OR auth <client_id> <client_secret> <tenant_id>",
		Completer: func(args []string) []string {
			if len(args) == 0 {
				return getDirectory()
			}
			return []string{}
		},
		Func: auth,
	})

	getCmd := ishell.Cmd{
		Name: "get",
		Help: "Usage: get <users|resource> <arguments...>",
	}

	getCmd.AddCmd(&ishell.Cmd{
		Name: "users",
		Help: "Usage: get users <output_file>",
		Completer: func(args []string) []string {
			if len(args) == 0 {
				return getDirectory()
			}
			return []string{}
		},
		Func: getUser,
	})

	getCmd.AddCmd(&ishell.Cmd{
		Name: "resource",
		Help: "Usage: get resource <type> <input_file> <output_file> [worker_count=4]",
		Completer: func(args []string) []string {
			if len(args) == 0 {
				return []string{"authenticate"}
			} else if len(args) < 3 {
				return getDirectory()
			}
			return []string{}
		},
		Func: getResource,
	})

	shell.AddCmd(&getCmd)

	if len(os.Args) > 1 {
		err := shell.Process(os.Args[1:]...)
		if err != nil {
			fmt.Println(err.Error())
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

func getUser(c *ishell.Context) {
	start := time.Now()

	if len(c.Args) != 1 {
		fmt.Println(c.Cmd.Help)
		return
	}

	g := c.Get("api").(*api.GraphAPI)
	if !g.IsInitiated() {
		fmt.Println("Error: Use 'auth' command to authenticate the API before use")
		return
	}

	result := g.GetUsers()

	if dumpFile(result, c.Args[0], true) {
		fmt.Printf("Success: Processed %v entries in %.2f seconds\n", len(result), time.Since(start).Seconds())
	}
}

func getResource(c *ishell.Context) {
	start := time.Now()

	if len(c.Args) != 3 && len(c.Args) != 4 {
		fmt.Println(c.Cmd.Help)
		return
	}

	workerCount := 4
	if len(c.Args) == 4 {
		i, err := strconv.ParseUint(c.Args[3], 10, 32)
		if err != nil {
			fmt.Println("Error: Invalid worker node count")
			return
		}
		workerCount = int(i)
	}

	g := c.Get("api").(*api.GraphAPI)
	if !g.IsInitiated() {
		fmt.Println("Error: Use 'auth' command to authenticate the API before use")
		return
	}

	userIds := getUserIds(c.Args[1])
	if userIds == nil {
		return
	}

	var result map[string][]interface{}

	switch c.Args[0] {
	case "authenticate":
		cfg := &users.ItemAuthenticationMethodsRequestBuilderGetRequestConfiguration{}
		result = api.GetResourceByUserIdsConcurrent(c, userIds, "authentication/methods", cfg, workerCount, 20)
	default:
		fmt.Println("Error: Unknown resource")
		return
	}

	if dumpFile(result, c.Args[2], true) {
		fmt.Printf("Success: Processed %v entries in %.2f seconds\n", len(userIds), time.Since(start).Seconds())
	}
}

func getDirectory() []string {
	entries, err := os.ReadDir("./")
	if err != nil {
		return []string{}
	}
	var es []string
	for _, e := range entries {
		es = append(es, e.Name())
	}
	return es
}

func getUserIds(file string) []string {
	userInput, err := os.ReadFile(file)
	if err != nil {
		fmt.Println("Error: Failed to read the input file")
		return nil
	}

	var userJSON []map[string]interface{}
	err = json.Unmarshal(userInput, &userJSON)
	if err != nil {
		fmt.Println("Error: Failed to parse input JSON")
		return nil
	}

	var result []string
	for _, v := range userJSON {
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
