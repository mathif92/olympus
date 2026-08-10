package pkg

import "k8s.io/apimachinery/pkg/api/resource"

func resourceMustParse(s string) resource.Quantity {
	return resource.MustParse(s)
}
