package cognition

import "testing"

func TestDecodeRouterOutputAcceptsOnlyDirectOrFull(t *testing.T) {
	for _, route := range []Route{RouteDirect, RouteFull} {
		output, err := DecodeRouterOutput([]byte(`{"route":"` + string(route) + `"}`))
		if err != nil {
			t.Fatalf("route %s: %v", route, err)
		}
		if output.Route != route {
			t.Fatalf("route = %q, want %q", output.Route, route)
		}
	}
}

func TestDecodeRouterOutputRejectsEverythingOutsideContract(t *testing.T) {
	for name, payload := range map[string]string{
		"unknown route":         `{"route":"maybe"}`,
		"wrong case":            `{"route":"FULL"}`,
		"extra field":           `{"route":"full","depth":3}`,
		"duplicate route":       `{"route":"full","route":"direct"}`,
		"multiple values":       `{"route":"full"} {"route":"direct"}`,
		"plain text":            `full`,
		"null":                  `null`,
		"missing route":         `{}`,
		"route with whitespace": `{"route":" full "}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeRouterOutput([]byte(payload)); err == nil {
				t.Fatalf("payload %s was accepted", payload)
			}
		})
	}
}

func TestRouterInputRequiresUserInput(t *testing.T) {
	if err := (RouterInput{}).Validate(); err == nil {
		t.Fatal("empty router input was accepted")
	}
	if err := (RouterInput{UserInput: "hello"}).Validate(); err != nil {
		t.Fatal(err)
	}
}
