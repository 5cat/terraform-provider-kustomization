package kustomize

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/helper/validation"

	"sigs.k8s.io/kustomize/api/filesys"
)

func dataSourceKustomization() *schema.Resource {
	return &schema.Resource{
		Read: kustomizationBuild,

		Schema: map[string]*schema.Schema{
			"path": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
			},
			"load_restrictor": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				Default:  "",
				ValidateFunc: validation.StringInSlice(
					[]string{"none", ""},
					false,
				),
			},
			"ids": &schema.Schema{
				Type:     schema.TypeSet,
				Computed: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
				Set:      idSetHash,
			},
			"ids_prio": &schema.Schema{
				Type:     schema.TypeList,
				Computed: true,
				MinItems: 3,
				MaxItems: 3,
				Elem: &schema.Schema{
					Type: schema.TypeSet,
					Set:  idSetHash,
				},
			},
			"manifests": &schema.Schema{
				Type:     schema.TypeMap,
				Computed: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},
		},
	}
}

func kustomizationBuild(d *schema.ResourceData, m interface{}) error {
	path := d.Get("path").(string)
	load_restrictor := d.Get("load_restrictor").(string)

	fSys := filesys.MakeFsOnDisk()

	// mutex as tmp workaround for upstream bug
	// https://github.com/kubernetes-sigs/kustomize/issues/3659
	mu := m.(*Config).Mutex
	mu.Lock()
	rm, err := runKustomizeBuild(fSys, path, load_restrictor)
	mu.Unlock()
	if err != nil {
		return fmt.Errorf("kustomizationBuild: %s", err)
	}

	return setGeneratedAttributes(d, rm)
}
