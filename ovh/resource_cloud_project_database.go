package ovh

import (
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/ovh/terraform-provider-ovh/ovh/helpers"
)

func resourceCloudProjectDatabase() *schema.Resource {
	return &schema.Resource{
		Create: resourceCloudProjectDatabaseCreate,
		Read:   resourceCloudProjectDatabaseRead,
		Delete: resourceCloudProjectDatabaseDelete,
		Update: resourceCloudProjectDatabaseUpdate,

		Importer: &schema.ResourceImporter{
			State: resourceCloudProjectDatabaseImportState,
		},

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(20 * time.Minute),
			Update: schema.DefaultTimeout(40 * time.Minute),
			Delete: schema.DefaultTimeout(20 * time.Minute),
		},

		Schema: map[string]*schema.Schema{
			"service_name": {
				Type:        schema.TypeString,
				ForceNew:    true,
				Required:    true,
				DefaultFunc: schema.EnvDefaultFunc("OVH_CLOUD_PROJECT_SERVICE", nil),
			},
			"description": {
				Type:        schema.TypeString,
				Description: "Description of the cluster",
				Optional:    true,
			},
			"engine": {
				Type:        schema.TypeString,
				Description: "Name of the engine of the service",
				ForceNew:    true,
				Required:    true,
			},
			"flavor": {
				Type:        schema.TypeString,
				Description: "The node flavor used for this cluster",
				Required:    true,
			},
			"kafka_rest_api": {
				Type:        schema.TypeBool,
				Description: "Defines whether the REST API is enabled on a kafka cluster",
				Optional:    true,
				DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
					return d.Get("engine").(string) != "kafka" || new == old
				},
			},
			"nodes": {
				Type:        schema.TypeList,
				Description: "List of nodes composing the service",
				Required:    true,
				MinItems:    1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"network_id": {
							Type:        schema.TypeString,
							Description: "Private network ID in which the node is",
							ForceNew:    true,
							Optional:    true,
						},
						"region": {
							Type:        schema.TypeString,
							Description: "Region of the node",
							ForceNew:    true,
							Required:    true,
						},
						"subnet_id": {
							Type:        schema.TypeString,
							Description: "Private subnet ID in which the node is",
							Optional:    true,
						},
					},
				},
			},
			"opensearch_acls_enabled": {
				Type:        schema.TypeBool,
				Description: "Defines whether the acls are enabled on an Opensearch cluster",
				Optional:    true,
				DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
					return d.Get("engine").(string) != "opensearch" || new == old
				},
			},
			"plan": {
				Type:        schema.TypeString,
				Description: "Plan of the cluster",
				Required:    true,
			},
			"version": {
				Type:        schema.TypeString,
				Description: "Version of the engine deployed on the cluster",
				Required:    true,
			},

			//Computed
			"backup_time": {
				Type:        schema.TypeString,
				Description: "Time on which backups start every day",
				Computed:    true,
			},
			"created_at": {
				Type:        schema.TypeString,
				Description: "Date of the creation of the cluster",
				Computed:    true,
			},
			"endpoints": {
				Type:        schema.TypeList,
				Description: "List of all endpoints of the service",
				Computed:    true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"component": {
							Type:        schema.TypeString,
							Description: "Type of component the URI relates to",
							Computed:    true,
						},
						"domain": {
							Type:        schema.TypeString,
							Description: "Domain of the cluster",
							Computed:    true,
						},
						"path": {
							Type:        schema.TypeString,
							Description: "Path of the endpoint",
							Computed:    true,
						},
						"port": {
							Type:        schema.TypeInt,
							Description: "Connection port for the endpoint",
							Computed:    true,
						},
						"scheme": {
							Type:        schema.TypeString,
							Description: "Scheme used to generate the URI",
							Computed:    true,
						},
						"ssl": {
							Type:        schema.TypeBool,
							Description: "Defines whether the endpoint uses SSL",
							Computed:    true,
						},
						"ssl_mode": {
							Type:        schema.TypeString,
							Description: "SSL mode used to connect to the service if the SSL is enabled",
							Computed:    true,
						},
						"uri": {
							Type:        schema.TypeString,
							Description: "URI of the endpoint",
							Computed:    true,
						},
					},
				},
			},
			"maintenance_time": {
				Type:        schema.TypeString,
				Description: "Time on which maintenances can start every day",
				Computed:    true,
			},
			"network_type": {
				Type:        schema.TypeString,
				Description: "Type of network of the cluster",
				Computed:    true,
			},
			"status": {
				Type:        schema.TypeString,
				Description: "Current status of the cluster",
				Computed:    true,
			},
		},
	}
}

func resourceCloudProjectDatabaseImportState(d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	givenId := d.Id()
	n := 3
	splitId := strings.SplitN(givenId, "/", n)
	if len(splitId) != n {
		return nil, fmt.Errorf("Import Id is not service_name/engine/databaseId formatted")
	}
	serviceName := splitId[0]
	engine := splitId[1]
	id := splitId[2]
	d.SetId(id)
	d.Set("engine", engine)
	d.Set("service_name", serviceName)

	results := make([]*schema.ResourceData, 1)
	results[0] = d
	return results, nil
}

func resourceCloudProjectDatabaseCreate(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*Config)
	serviceName := d.Get("service_name").(string)
	engine := d.Get("engine").(string)

	endpoint := fmt.Sprintf("/cloud/project/%s/database/%s",
		url.PathEscape(serviceName),
		url.PathEscape(engine),
	)
	err, params := (&CloudProjectDatabaseCreateOpts{}).FromResource(d)
	if err != nil {
		return fmt.Errorf("multi region cluster not available yet : %q", err)
	}
	res := &CloudProjectDatabaseResponse{}

	log.Printf("[DEBUG] Will create Database: %+v", params)
	err = config.OVHClient.Post(endpoint, params, res)
	if err != nil {
		return fmt.Errorf("calling Post %s with params %+v:\n\t %q", endpoint, params, err)
	}

	log.Printf("[DEBUG] Waiting for database %s to be READY", res.Id)
	err = waitForCloudProjectDatabaseReady(config.OVHClient, serviceName, engine, res.Id, d.Timeout(schema.TimeoutCreate))
	if err != nil {
		return fmt.Errorf("timeout while waiting database %s to be READY: %w", res.Id, err)
	}
	log.Printf("[DEBUG] database %s is READY", res.Id)

	d.SetId(res.Id)

	if (engine == "kafka" && d.Get("kafka_rest_api").(bool)) || (engine == "opensearch" && d.Get("opensearch_acls_enabled").(bool)) {
		return resourceCloudProjectDatabaseUpdate(d, meta)
	}

	return resourceCloudProjectDatabaseRead(d, meta)
}

func resourceCloudProjectDatabaseRead(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*Config)
	serviceName := d.Get("service_name").(string)
	engine := d.Get("engine").(string)

	serviceEndpoint := fmt.Sprintf("/cloud/project/%s/database/%s/%s",
		url.PathEscape(serviceName),
		url.PathEscape(engine),
		url.PathEscape(d.Id()),
	)
	res := &CloudProjectDatabaseResponse{}

	log.Printf("[DEBUG] Will read database %s from project: %s", d.Id(), serviceName)
	if err := config.OVHClient.Get(serviceEndpoint, res); err != nil {
		return helpers.CheckDeleted(d, err, serviceEndpoint)
	}

	nodesEndpoint := fmt.Sprintf("%s/node", serviceEndpoint)
	nodeList := &[]string{}
	if err := config.OVHClient.Get(nodesEndpoint, nodeList); err != nil {
		return fmt.Errorf("unable to get database %s nodes: %v", res.Id, err)
	}

	if len(*nodeList) == 0 {
		return fmt.Errorf("no node found for database %s", res.Id)
	}
	nodeEndpoint := fmt.Sprintf("%s/%s", nodesEndpoint, url.PathEscape((*nodeList)[0]))
	node := &CloudProjectDatabaseNodes{}
	if err := config.OVHClient.Get(nodeEndpoint, node); err != nil {
		return fmt.Errorf("unable to get database %s node %s: %v", res.Id, (*nodeList)[0], err)
	}

	res.Region = node.Region

	for k, v := range res.ToMap() {
		if k != "id" {
			d.Set(k, v)
		} else {
			d.SetId(fmt.Sprint(v))
		}
	}

	log.Printf("[DEBUG] Read Database %+v", res)
	return nil
}

func resourceCloudProjectDatabaseUpdate(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*Config)
	serviceName := d.Get("service_name").(string)
	engine := d.Get("engine").(string)

	endpoint := fmt.Sprintf("/cloud/project/%s/database/%s/%s",
		url.PathEscape(serviceName),
		url.PathEscape(engine),
		url.PathEscape(d.Id()),
	)
	err, params := (&CloudProjectDatabaseUpdateOpts{}).FromResource(d)
	if err != nil {
		return fmt.Errorf("multi region cluster not available yet : %q", err)
	}
	log.Printf("[DEBUG] Will update database: %+v", params)
	err = config.OVHClient.Put(endpoint, params, nil)
	if err != nil {
		return fmt.Errorf("calling Put %s with params %v:\n\t %q", endpoint, params, err)
	}

	log.Printf("[DEBUG] Waiting for database %s to be READY", d.Id())
	err = waitForCloudProjectDatabaseReady(config.OVHClient, serviceName, engine, d.Id(), d.Timeout(schema.TimeoutUpdate))
	if err != nil {
		return fmt.Errorf("timeout while waiting database %s to be READY: %w", d.Id(), err)
	}
	log.Printf("[DEBUG] database %s is READY", d.Id())

	return resourceCloudProjectDatabaseRead(d, meta)
}

func resourceCloudProjectDatabaseDelete(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*Config)
	serviceName := d.Get("service_name").(string)
	engine := d.Get("engine").(string)

	endpoint := fmt.Sprintf("/cloud/project/%s/database/%s/%s",
		url.PathEscape(serviceName),
		url.PathEscape(engine),
		url.PathEscape(d.Id()),
	)

	log.Printf("[DEBUG] Will delete database %s from project: %s", d.Id(), serviceName)
	err := config.OVHClient.Delete(endpoint, nil)
	if err != nil {
		return helpers.CheckDeleted(d, err, endpoint)
	}

	log.Printf("[DEBUG] Waiting for database %s to be DELETED", d.Id())
	err = waitForCloudProjectDatabaseDeleted(config.OVHClient, serviceName, engine, d.Id(), d.Timeout(schema.TimeoutDelete))
	if err != nil {
		return fmt.Errorf("timeout while waiting database %s to be DELETED: %v", d.Id(), err)
	}
	log.Printf("[DEBUG] database %s is DELETED", d.Id())

	d.SetId("")

	return nil
}
