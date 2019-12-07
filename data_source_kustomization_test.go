package main

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/helper/resource"
)

func TestAccDataSourceKustomization_basic(t *testing.T) {

	resource.Test(t, resource.TestCase{
		//PreCheck:  func() { testAccPreCheck(t) },
		Providers: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccDataSourceKustomizationConfig_basic("test_kustomizations/basic/initial"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.kustomization.test", "id"),
					resource.TestCheckResourceAttrSet("data.kustomization.test", "path"),
					resource.TestCheckResourceAttr("data.kustomization.test", "path", "test_kustomizations/basic/initial"),
					resource.TestCheckResourceAttr("data.kustomization.test", "ids.#", "3"),
					resource.TestCheckResourceAttr("data.kustomization.test", "manifests.%", "3"),
				),
			},
		},
	})
}

func testAccDataSourceKustomizationConfig_basic(path string) string {
	return fmt.Sprintf(`
data "kustomization" "test" {
  path = "%s"
}
`, path)
}
