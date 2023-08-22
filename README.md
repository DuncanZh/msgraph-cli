# Microsoft Graph API CLI

`msgraph-cli` is a command-line interface (CLI) tool designed to interact with the Microsoft Graph API written in Go. It provides an interactive shell, allowing users to authenticate with secrets and retrieve specified resources.

## Features

- Authenticate with Microsoft Graph API using credentials or a file
- List specifies resources
- Retrieve specified resources concurrently
- Support expand argument to fetch additional resources
- Provide Interactive shell for ease of use
- Store results to JSON files for future use

## Usage

Run the program by executing:

```
go run main.go
```

### Commands

#### 1. `auth`

Authenticate with the Microsoft Graph API with secret. The `auth` command accepts literals or a JSON credential file containing the `clientId`, `clientSecret`, and `tenantId`.

```
auth <credential_file>
auth <client_id> <client_secret> <tenant_id>
```

#### 2. `list`

List the specific resource and dump to `output_file`.

This command accepts an optional argument `expand` that fetches one additional resource related to the original resource. Please refer to the official document for avaliable resources.

```
list <resource> <output_file> [expand]
```

#### 3. `get`

Get specified resources by user ids from `user_file` and dump to `output_file`. The input file must be a valid JSON array containing users with ids. Please refer to the output of `list users`.

This command accepts an optional argument `expand` that fetches one additional resource related to the original resource. Please refer to the official document for available resources.

```
get <resource> <user_file> <output_file> [expand]
```

## Examples

**Authenticate with a credentials file**:

```
auth credentials.json
```

*credentials.json*

```json
{
    "clientId": "xxx",
    "clientSecret": "xxx",
    "tenantId": "xxx"
}
```

**Authenticate with client ID, secret, and tenant ID**:

```
auth my_client_id my_client_secret my_tenant_id
```

**Get users**:

```
get users users.json
```

*users.json*

```json
[  
 {
  "displayName": "User1",  
  "id": "xxx"
  },
  {
  "displayName": "User2",  
  "id": "yyy"
  }
]
```

**Get resources**:

```
get resource authenticate users.json output.json
```

*output.json*

```json
{  
 "xxx": [  
  {
   "createdDateTime": "2020-10-28T20:47:31Z",  
   "id": "aaa",  
   "odataType": "#microsoft.graph.passwordAuthenticationMethod"  
  },
  {
	"emailAddress": "foo@bar.com",  
	"id": "bbb",  
	"odataType": "#microsoft.graph.emailAuthenticationMethod"  
  }
 ],
 "yyy": [  
  {
   "createdDateTime": "2023-08-06T17:13:18Z",  
   "id": "ccc",  
   "odataType": "#microsoft.graph.passwordAuthenticationMethod"  
  }  
 ]
}
```

## References

1. **Microsoft Graph API**: 
	- [Official Documentation](https://docs.microsoft.com/en-us/graph/overview)
	- [GitHub Repository](https://github.com/microsoftgraph/msgraph-sdk-go)
2. **Interactive Shell Library (ishell)**: 
	- [GitHub Repository](https://github.com/abiosoft/ishell)
