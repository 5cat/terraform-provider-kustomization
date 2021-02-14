package kustomize

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"

	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/helper/validation"

	"sigs.k8s.io/kustomize/api/filesys"
	"sigs.k8s.io/kustomize/api/types"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

func dataSourceKustomizationOverlay() *schema.Resource {
	return &schema.Resource{
		Read: kustomizationOverlay,

		Schema: map[string]*schema.Schema{
			"common_annotations": &schema.Schema{
				Type:     schema.TypeMap,
				Optional: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},
			"common_labels": &schema.Schema{
				Type:     schema.TypeMap,
				Optional: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},
			"components": &schema.Schema{
				Type:     schema.TypeList,
				Optional: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
			"config_map_generator": &schema.Schema{
				Type:     schema.TypeList,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"name": {
							Type:     schema.TypeString,
							Optional: true,
						},
						"behavior": {
							Type:     schema.TypeString,
							Optional: true,
							ValidateFunc: validation.StringInSlice(
								[]string{"create", "replace", "merge"},
								false,
							),
						},
						"envs": {
							Type:     schema.TypeList,
							Optional: true,
							Elem:     &schema.Schema{Type: schema.TypeString},
						},
						"files": {
							Type:     schema.TypeList,
							Optional: true,
							Elem:     &schema.Schema{Type: schema.TypeString},
						},
						"literals": {
							Type:     schema.TypeList,
							Optional: true,
							Elem:     &schema.Schema{Type: schema.TypeString},
						},
					},
				},
			},
			"crds": &schema.Schema{
				Type:     schema.TypeList,
				Optional: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
			"images": &schema.Schema{
				Type:     schema.TypeList,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"name": {
							Type:     schema.TypeString,
							Optional: true,
						},
						"new_name": {
							Type:     schema.TypeString,
							Optional: true,
						},
						"new_tag": {
							Type:     schema.TypeString,
							Optional: true,
						},
						"digest": {
							Type:     schema.TypeString,
							Optional: true,
						},
					},
				},
			},
			"name_prefix": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
			},
			"namespace": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
			},
			"name_suffix": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
			},
			"replicas": &schema.Schema{
				Type:     schema.TypeList,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"name": {
							Type:     schema.TypeString,
							Optional: true,
						},
						"count": {
							Type:     schema.TypeInt,
							Optional: true,
						},
					},
				},
			},
			"resources": &schema.Schema{
				Type:     schema.TypeList,
				Optional: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
			"secret_generator": &schema.Schema{
				Type:     schema.TypeList,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"name": {
							Type:     schema.TypeString,
							Optional: true,
						},
						"type": {
							Type:     schema.TypeString,
							Optional: true,
						},
						"envs": {
							Type:     schema.TypeList,
							Optional: true,
							Elem:     &schema.Schema{Type: schema.TypeString},
						},
						"files": {
							Type:     schema.TypeList,
							Optional: true,
							Elem:     &schema.Schema{Type: schema.TypeString},
						},
						"literals": {
							Type:     schema.TypeList,
							Optional: true,
							Elem:     &schema.Schema{Type: schema.TypeString},
						},
					},
				},
			},
			"ids": &schema.Schema{
				Type:     schema.TypeSet,
				Computed: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
				Set:      idSetHash,
			},
			"manifests": &schema.Schema{
				Type:     schema.TypeMap,
				Computed: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},
		},
	}
}

func convertListInterfaceToListString(in []interface{}) (out []string) {
	for _, v := range in {
		out = append(out, v.(string))
	}
	return out
}

func convertMapStringInterfaceToMapStringString(in map[string]interface{}) (out map[string]string) {
	out = make(map[string]string)
	for k, v := range in {
		out[k] = v.(string)
	}
	return out
}

func getKustomization(d *schema.ResourceData) (k types.Kustomization) {
	k.TypeMeta = types.TypeMeta{
		APIVersion: "kustomize.config.k8s.io/v1beta1",
		Kind:       "Kustomization",
	}

	if d.Get("common_annotations") != nil {
		k.CommonAnnotations = convertMapStringInterfaceToMapStringString(
			d.Get("common_annotations").(map[string]interface{}),
		)
	}

	if d.Get("common_labels") != nil {
		k.CommonLabels = convertMapStringInterfaceToMapStringString(
			d.Get("common_labels").(map[string]interface{}),
		)
	}

	if d.Get("components") != nil {
		k.Components = convertListInterfaceToListString(
			d.Get("components").([]interface{}),
		)
	}

	if d.Get("config_map_generator") != nil {
		cmgs := d.Get("config_map_generator").([]interface{})
		for i := range cmgs {
			if cmgs[i] == nil {
				continue
			}

			cmg := cmgs[i].(map[string]interface{})
			cma := types.ConfigMapArgs{}

			cma.Name = cmg["name"].(string)

			cma.Behavior = cmg["behavior"].(string)

			cma.EnvSources = convertListInterfaceToListString(
				cmg["envs"].([]interface{}),
			)

			cma.LiteralSources = convertListInterfaceToListString(
				cmg["literals"].([]interface{}),
			)

			cma.FileSources = convertListInterfaceToListString(
				cmg["files"].([]interface{}),
			)

			k.ConfigMapGenerator = append(k.ConfigMapGenerator, cma)
		}
	}

	if d.Get("crds") != nil {
		k.Crds = convertListInterfaceToListString(
			d.Get("crds").([]interface{}),
		)
	}

	if d.Get("images") != nil {
		imgs := d.Get("images").([]interface{})
		for i := range imgs {
			if imgs[i] == nil {
				continue
			}

			img := imgs[i].(map[string]interface{})
			kimg := types.Image{}

			kimg.Name = img["name"].(string)
			kimg.NewName = img["new_name"].(string)
			kimg.NewTag = img["new_tag"].(string)
			kimg.Digest = img["digest"].(string)

			k.Images = append(k.Images, kimg)
		}
	}

	if d.Get("replicas") != nil {
		rs := d.Get("replicas").([]interface{})
		for i := range rs {
			if rs[i] == nil {
				continue
			}

			img := rs[i].(map[string]interface{})
			r := types.Replica{}

			r.Name = img["name"].(string)
			r.Count = int64(img["count"].(int))

			k.Replicas = append(k.Replicas, r)
		}
	}

	if d.Get("name_prefix") != nil {
		k.NamePrefix = d.Get("name_prefix").(string)
	}

	if d.Get("namespace") != nil {
		k.Namespace = d.Get("namespace").(string)
	}

	if d.Get("name_suffix") != nil {
		k.NameSuffix = d.Get("name_suffix").(string)
	}

	if d.Get("resources") != nil {
		k.Resources = convertListInterfaceToListString(
			d.Get("resources").([]interface{}),
		)
	}

	if d.Get("secret_generator") != nil {
		sg := d.Get("secret_generator").([]interface{})
		for i := range sg {
			if sg[i] == nil {
				continue
			}

			s := sg[i].(map[string]interface{})
			sa := types.SecretArgs{}

			sa.Name = s["name"].(string)
			sa.Type = s["type"].(string)

			sa.EnvSources = convertListInterfaceToListString(
				s["envs"].([]interface{}),
			)

			sa.LiteralSources = convertListInterfaceToListString(
				s["literals"].([]interface{}),
			)

			sa.FileSources = convertListInterfaceToListString(
				s["files"].([]interface{}),
			)

			k.SecretGenerator = append(k.SecretGenerator, sa)
		}
	}

	return k
}

func kustomizationOverlay(d *schema.ResourceData, m interface{}) error {
	k := getKustomization(d)

	fSys := filesys.MakeFsOnDisk()

	var b bytes.Buffer
	ye := yaml.NewEncoder(io.Writer(&b))
	ye.Encode(k)
	ye.Close()
	data, _ := ioutil.ReadAll(io.Reader(&b))

	fSys.WriteFile("Kustomization", data)
	defer fSys.RemoveAll("Kustomization")

	rm, err := runKustomizeBuild(fSys, ".")
	if err != nil {
		return fmt.Errorf("buildKustomizeOverlay: %s", err)
	}

	return setGeneratedAttributes(d, rm)
}
