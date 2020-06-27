package config

import (
	"io/ioutil"
	"os"
	"testing"
)

func TestNewParser_ok(t *testing.T) {
	configPath := "/tmp/ok.json"
	configContent := []byte(`{
    "version": 2,
    "name": "My lovely gateway",
    "port": 8080,
    "cache_ttl": "3600s",
    "timeout": "3s",
    "tls": {
		"public_key":  "cert.pem",
		"private_key": "key.pem"
	},
    "endpoints": [
        {
            "endpoint": "/github",
            "method": "GET",
            "extra_config" : {"user":"test","hits":6,"parents":["gomez","morticia"]},
            "backend": [
                {
                    "host": [
                        "https://api.github.com"
                    ],
                    "url_pattern": "/",
                    "whitelist": [
                        "authorizations_url",
                        "code_search_url"
                    ],
                    "extra_config" : {"user":"test","hits":6,"parents":["gomez","morticia"]}
                }
            ]
        },
        {
            "endpoint": "/supu",
            "method": "GET",
            "concurrent_calls": 3,
            "backend": [
                {
                    "host": [
                        "http://127.0.0.1:8080"
                    ],
                    "url_pattern": "/__debug/supu"
                }
            ]
        },
        {
            "endpoint": "/combination/{id}",
            "method": "GET",
            "backend": [
                {
                    "group": "first_post",
                    "host": [
                        "https://jsonplaceholder.typicode.com"
                    ],
                    "url_pattern": "/posts/{id}",
                    "blacklist": [
                        "userId"
                    ]
                },
                {
                    "host": [
                        "https://jsonplaceholder.typicode.com"
                    ],
                    "url_pattern": "/users/{id}",
                    "mapping": {
                        "email": "personal_email"
                    }
                }
            ]
        }
    ],
    "extra_config" : {"user":"test","hits":6,"parents":["gomez","morticia"]}
}`)
	if err := ioutil.WriteFile(configPath, configContent, 0644); err != nil {
		t.FailNow()
	}

	serviceConfig, err := NewParser().Parse(configPath)

	if err != nil {
		t.Error("Unexpected error. Got", err.Error())
	}
	testExtraConfig(serviceConfig.ExtraConfig, t)

	if endpoints := len(serviceConfig.Endpoints); endpoints != 3 {
		t.Errorf("Unexpected number of endpoints: %d", endpoints)
		return
	}

	endpoint := serviceConfig.Endpoints[0]
	endpointExtraConfiguration := endpoint.ExtraConfig

	if endpointExtraConfiguration != nil {
		testExtraConfig(endpointExtraConfiguration, t)
	} else {
		t.Error("Extra config is not present in EndpointConfig")
	}

	if serviceConfig.TLS == nil {
		t.Error("TLS config not present")
	} else {
		if serviceConfig.TLS.PublicKey != "cert.pem" {
			t.Error("Unexpected TLS Public key")
		}
		if serviceConfig.TLS.PrivateKey != "key.pem" {
			t.Error("Unexpected TLS Private key")
		}
	}

	backend := endpoint.Backend[0]
	backendExtraConfiguration := backend.ExtraConfig
	if backendExtraConfiguration != nil {
		testExtraConfig(backendExtraConfiguration, t)
	} else {
		t.Error("Extra config is not present in BackendConfig")
	}

	if err := os.Remove(configPath); err != nil {
		t.FailNow()
	}
}
func TestNewParserWithAddress_ok(t *testing.T) {
	configPath := "/tmp/ok.json"
	configContent := []byte(`{
    "version": 2,
	"name": "My lovely gateway",
	"address": "localhost:9090",
    "port": 8080,
    "cache_ttl": "3600s",
    "timeout": "3s",
    "endpoints": [
        {
            "endpoint": "/github",
            "method": "GET",
            "extra_config" : {"user":"test","hits":6,"parents":["gomez","morticia"]},
            "backend": [
                {
                    "host": [
                        "https://api.github.com"
                    ],
                    "url_pattern": "/",
                    "whitelist": [
                        "authorizations_url",
                        "code_search_url"
                    ],
                    "extra_config" : {"user":"test","hits":6,"parents":["gomez","morticia"]}
                }
            ]
        },
        {
            "endpoint": "/supu",
            "method": "GET",
            "concurrent_calls": 3,
            "backend": [
                {
                    "host": [
                        "http://127.0.0.1:8080"
                    ],
                    "url_pattern": "/__debug/supu"
                }
            ]
        },
        {
            "endpoint": "/combination/{id}",
            "method": "GET",
            "backend": [
                {
                    "group": "first_post",
                    "host": [
                        "https://jsonplaceholder.typicode.com"
                    ],
                    "url_pattern": "/posts/{id}",
                    "blacklist": [
                        "userId"
                    ]
                },
                {
                    "host": [
                        "https://jsonplaceholder.typicode.com"
                    ],
                    "url_pattern": "/users/{id}",
                    "mapping": {
                        "email": "personal_email"
                    }
                }
            ]
        }
    ],
    "extra_config" : {"user":"test","hits":6,"parents":["gomez","morticia"]}
}`)
	if err := ioutil.WriteFile(configPath, configContent, 0644); err != nil {
		t.FailNow()
	}

	serviceConfig, err := NewParser().Parse(configPath)

	if serviceConfig.Port == 8080 {
		t.Errorf("Expected `address` to bind stronger than `port`.`port` want %v `port` got %v", 9090, serviceConfig.Port)
	}

	if err != nil {
		t.Error("Unexpected error. Got", err.Error())
	}
	testExtraConfig(serviceConfig.ExtraConfig, t)

	endpoint := serviceConfig.Endpoints[0]
	endpointExtraConfiguration := endpoint.ExtraConfig

	if endpointExtraConfiguration != nil {
		testExtraConfig(endpointExtraConfiguration, t)
	} else {
		t.Error("Extra config is not present in EndpointConfig")
	}

	backend := endpoint.Backend[0]
	backendExtraConfiguration := backend.ExtraConfig
	if backendExtraConfiguration != nil {
		testExtraConfig(backendExtraConfiguration, t)
	} else {
		t.Error("Extra config is not present in BackendConfig")
	}

	if err := os.Remove(configPath); err != nil {
		t.FailNow()
	}
}

func TestNewParser_errorMessages(t *testing.T) {
	for _, configContent := range []struct {
		name    string
		path    string
		content []byte
		expErr  string
	}{
		{
			name:    "case0",
			path:    "/tmp/ok.json",
			content: []byte(`{`),
			expErr:  "'/tmp/ok.json': unexpected end of JSON input, offset: 1, row: 0, col: 1",
		},
		{
			name:    "case1",
			path:    "/tmp/ok.json",
			content: []byte(`>`),
			expErr:  "'/tmp/ok.json': invalid character '>' looking for beginning of value, offset: 1, row: 0, col: 1",
		},
		{
			name:    "case2",
			path:    "/tmp/ok.json",
			content: []byte(`"`),
			expErr:  "'/tmp/ok.json': unexpected end of JSON input, offset: 1, row: 0, col: 1",
		},
		{
			name:    "case3",
			path:    "/tmp/ok.json",
			content: []byte(``),
			expErr:  "'/tmp/ok.json': unexpected end of JSON input, offset: 0, row: 0, col: 0",
		},
		{
			name:    "case4",
			path:    "/tmp/ok.json",
			content: []byte(`[{}]`),
			expErr:  "'/tmp/ok.json': json: cannot unmarshal array into Go value of type config.parseableServiceConfig, offset: 1, row: 0, col: 1",
		},
		{
			name:    "case5",
			path:    "/tmp/ok.json",
			content: []byte(`42`),
			expErr:  "'/tmp/ok.json': json: cannot unmarshal number into Go value of type config.parseableServiceConfig, offset: 2, row: 0, col: 2",
		},
		{
			name:    "case6",
			path:    "/tmp/ok.json",
			content: []byte("\r\n42"),
			expErr:  "'/tmp/ok.json': json: cannot unmarshal number into Go value of type config.parseableServiceConfig, offset: 4, row: 1, col: 2",
		},
		{
			name: "case7",
			path: "/tmp/ok.json",
			content: []byte(`{
	"version": 2,
	"name": "My lovely gateway",
	"port": 8080,
	"cache_ttl": 3600
	"timeout": "3s",
	"endpoints": []
}`),
			expErr: "'/tmp/ok.json': invalid character '\"' after object key:value pair, offset: 83, row: 5, col: 2",
		},
	} {
		t.Run(configContent.name, func(t *testing.T) {
			if err := ioutil.WriteFile(configContent.path, configContent.content, 0644); err != nil {
				t.Error(err)
				return
			}

			_, err := NewParser().Parse(configContent.path)
			if err == nil {
				t.Errorf("%s: Expecting error", configContent.name)
				return
			}
			if errMsg := err.Error(); errMsg != configContent.expErr {
				t.Errorf("%s: Unexpected error. Got '%s' want '%s'", configContent.name, errMsg, configContent.expErr)
				return
			}

			if err := os.Remove(configContent.path); err != nil {
				t.Errorf("%s: %s", err.Error(), configContent.name)
				return
			}
		})
	}
}

func testExtraConfig(extraConfig map[string]interface{}, t *testing.T) {
	userVar := extraConfig["user"]
	if userVar != "test" {
		t.Error("User in extra config is not test")
	}
	parents, ok := extraConfig["parents"].([]interface{})
	if !ok || parents[0] != "gomez" {
		t.Error("Parent 0 of user us not gomez")
	}
	if !ok || parents[1] != "morticia" {
		t.Error("Parent 1 of user us not morticia")
	}
}

func TestNewParser_unknownFile(t *testing.T) {
	_, err := NewParser().Parse("/nowhere/in/the/fs.json")
	if err == nil || err.Error() != "'/nowhere/in/the/fs.json' (open): no such file or directory" {
		t.Errorf("error expected. got '%v'", err)
	}
}

func TestNewParser_readingError(t *testing.T) {
	wrongConfigPath := "/tmp/reading.json"
	wrongConfigContent := []byte("{hello\ngo\n")
	if err := ioutil.WriteFile(wrongConfigPath, wrongConfigContent, 0644); err != nil {
		t.FailNow()
	}

	expected := "'/tmp/reading.json': invalid character 'h' looking for beginning of object key string, offset: 2, row: 0, col: 2"
	_, err := NewParser().Parse(wrongConfigPath)
	if err == nil || err.Error() != expected {
		t.Error("Error expected. Got", err)
	}
	if err = os.Remove(wrongConfigPath); err != nil {
		t.FailNow()
	}
}

func TestNewParser_initError(t *testing.T) {
	wrongConfigPath := "/tmp/unmarshall.json"
	wrongConfigContent := []byte("{\"a\":42}")
	if err := ioutil.WriteFile(wrongConfigPath, wrongConfigContent, 0644); err != nil {
		t.FailNow()
	}

	_, err := NewParser().Parse(wrongConfigPath)
	if err == nil || err.Error() != "'/tmp/unmarshall.json': Unsupported version: 0 (want: 2)" {
		t.Error("Error expected. Got", err)
	}
	if err = os.Remove(wrongConfigPath); err != nil {
		t.FailNow()
	}
}

func TestParserFunc(t *testing.T) {
	expected := ServiceConfig{Version: 42}
	result, err := ParserFunc(func(_ string) (ServiceConfig, error) { return expected, nil })("path/to/the/config/file")
	if err != nil {
		t.Error(err.Error())
	}
	if result.Version != expected.Version {
		t.Error("unexpected parsed config:", result)
	}
}
