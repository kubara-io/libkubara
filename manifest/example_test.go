package manifest_test

import (
	"fmt"
	"log"

	"github.com/kubara-io/libkubara/manifest"
)

func ExampleDecodeOneBytes() {
	yamlData := []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: game-config
  namespace: default
data:
  lives: "3"
`)

	obj, err := manifest.DecodeOneBytes(yamlData)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(obj.APIVersion())
	fmt.Println(obj.Kind())
	fmt.Println(obj.Namespace() + "/" + obj.Name())

	// Output:
	// v1
	// ConfigMap
	// default/game-config
}

func ExampleObject_Into() {
	yamlData := []byte(`
apiVersion: v1
kind: Service
metadata:
  name: web-service
spec:
  type: ClusterIP
  ports:
    - port: 80
      targetPort: 8080
`)

	obj, err := manifest.DecodeOneBytes(yamlData)
	if err != nil {
		log.Fatal(err)
	}

	type portSpec struct {
		Port       int `json:"port"`
		TargetPort int `json:"targetPort"`
	}
	type serviceSpec struct {
		Type  string     `json:"type"`
		Ports []portSpec `json:"ports"`
	}
	type serviceResource struct {
		Spec serviceSpec `json:"spec"`
	}

	var svc serviceResource
	if err := obj.Into(&svc); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Service type: %s, port: %d -> %d\n", svc.Spec.Type, svc.Spec.Ports[0].Port, svc.Spec.Ports[0].TargetPort)

	// Output:
	// Service type: ClusterIP, port: 80 -> 8080
}

func ExampleObject_NestedString() {
	yamlData := []byte(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx
spec:
  strategy:
    type: RollingUpdate
`)

	obj, err := manifest.DecodeOneBytes(yamlData)
	if err != nil {
		log.Fatal(err)
	}

	strategy, found, err := obj.NestedString("spec", "strategy", "type")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Found: %v, Strategy: %s\n", found, strategy)

	// Output:
	// Found: true, Strategy: RollingUpdate
}

func ExampleDecodeString() {
	streamYAML := `
apiVersion: v1
kind: ConfigMap
metadata:
  name: config-1
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: config-2
`

	objects, err := manifest.DecodeString(streamYAML)
	if err != nil {
		log.Fatal(err)
	}

	for _, obj := range objects {
		fmt.Println(obj.Name())
	}

	// Output:
	// config-1
	// config-2
}
