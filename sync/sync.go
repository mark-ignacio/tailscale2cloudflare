package sync

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	"github.com/rs/zerolog/log"
)

type tailnetDevicesResponse struct {
	Devices []tailnetDevice
}

// https://github.com/tailscale/tailscale/blob/main/api.md#tailnet-devices-get
type tailnetDevice struct {
	// there are other fields, but we only care about
	Hostname   string
	Addresses  []string
	Authorized bool
}

type dnsRecordsResponse struct {
	Success  bool
	Errors   []interface{}
	Messages []interface{}
	Result   []dnsRecord
}

type dnsRecord struct {
	ID       string
	Type     string
	Name     string
	Content  string
	ZoneName string `json:"zone_name"` // handy field we'll use
}

type Tailscale2CloudflareOptions struct {
	DryRun bool
}

func Tailscale2Cloudflare(tailscaleKey, tailscaleTailnet, cloudflareToken, cloudflareZone, cloudflareSubdomain string, opts *Tailscale2CloudflareOptions) error {
	if opts == nil {
		opts = &Tailscale2CloudflareOptions{}
	}
	// get tailscale devices
	devicesURL := fmt.Sprintf(
		"https://api.tailscale.com/api/v2/tailnet/%s/devices?fields=default",
		tailscaleTailnet,
	)
	request, _ := http.NewRequest("GET", devicesURL, nil)
	request.SetBasicAuth(tailscaleKey, "")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return fmt.Errorf("error performing Tailscale devices GET: %s", err)
	}
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("error reading Tailscale devices GET body: %s", err)
	}
	if response.StatusCode > 200 {
		return fmt.Errorf("non-200 response to Tailscale devices GET: %d: %s", response.StatusCode, body)
	}
	log.Debug().Interface("body", json.RawMessage(body)).Msg("GET devices")
	var devicesResponse tailnetDevicesResponse
	if err := json.Unmarshal(body, &devicesResponse); err != nil {
		return fmt.Errorf("error unmarshalling Tailscale devices GET as JSON: %s", err)
	}
	log.Debug().Interface("devices", devicesResponse.Devices).Msg("GET devices")
	// filter out authorized = false
	var hostname2IPv4s = map[string][]string{}
	for _, device := range devicesResponse.Devices {
		// does this happen? probably to someone
		if _, dupe := hostname2IPv4s[device.Hostname]; dupe {
			log.Warn().Str("hostname", device.Hostname).Msg("found multiple tailscale devices with the same hostname - the last listed device with this hostname will be used")
		}
		if !device.Authorized {
			log.Info().Str("hostname", device.Hostname).Msg("skipping unauthorized device")
			continue
		}
		// juuust ignore this one
		if device.Hostname == "hello.ipn.dev" {
			continue
		}
		hostname2IPv4s[device.Hostname] = device.Addresses
	}
	log.Debug().Interface("mapping", hostname2IPv4s).Msg("hostname -> IPv4 mappings")
	// get cloudflare records
	cfRecordsURLValues := url.Values{}
	cfRecordsURLValues.Set("per_page", "100")
	cfRecordsURLValues.Set("proxied", "false")
	cfRecordsURLValues.Set("type", "A")
	cfRecordsURL := fmt.Sprintf(
		"https://api.cloudflare.com/client/v4/zones/%s/dns_records?%s",
		cloudflareZone, cfRecordsURLValues.Encode(),
	)
	request, _ = http.NewRequest("GET", cfRecordsURL, nil)
	request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", cloudflareToken))
	request.Header.Set("Content-Type", "application/json")
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		return fmt.Errorf("error performing Cloudflare records GET: %s", err)
	}
	body, err = ioutil.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("error reading Cloudflare records GET body: %s", err)
	}
	if response.StatusCode > http.StatusOK {
		return fmt.Errorf("non-200 response to Cloudflare records GET: %d: %s", response.StatusCode, body)
	}
	log.Debug().Interface("body", json.RawMessage(body)).Msg("GET records")
	var recordsResponse dnsRecordsResponse
	if err := json.Unmarshal(body, &recordsResponse); err != nil {
		return fmt.Errorf("error unmarshalling Cloudflare records GET as JSON: %s", err)
	}
	log.Debug().Interface("records", recordsResponse.Result).Msg("GET records")
	if len(recordsResponse.Result) == 100 {
		log.Warn().Msg("recieved 100 Cloudflare DNS records - this does not currently paginate, so it's missing things")
	}
	// find out what needs updating and creating
	var (
		recordsByName = make(map[string][]dnsRecord, len(recordsResponse.Result))
		toUpdate      = map[string][]string{}
		toCreate      = map[string][]string{}
		toDelete      = map[string][]string{}
		zoneName      string
		recordSuffix  string
	)
	if len(recordsResponse.Result) == 0 {
		return fmt.Errorf("known TODO: handle getting the zone name from a separate request instead of skimming it off one of the record responses")
	}
	zoneName = recordsResponse.Result[0].ZoneName
	if cloudflareSubdomain != "" {
		recordSuffix = fmt.Sprintf("%s.%s", cloudflareSubdomain, zoneName)
	} else {
		recordSuffix = zoneName
	}
	// compute what needs updating
	for _, record := range recordsResponse.Result {
		recordsByName[record.Name] = append(recordsByName[record.Name], record)
		// compute what needs removing
		if strings.HasSuffix(record.Name, recordSuffix) {
			stripped := strings.ReplaceAll(record.Name, "."+recordSuffix, "")
			if hostname2IPv4s[stripped] == nil {
				toDelete[record.Name] = append(toDelete[record.Name], record.ID)
			}
		}
	}
	for hostname, ipv4s := range hostname2IPv4s {
		recordName := fmt.Sprintf("%s.%s", hostname, recordSuffix)
		// requires updating
		if existingRecords := recordsByName[recordName]; existingRecords != nil {
			if len(existingRecords) == 1 {
				if existingRecords[0].Content != ipv4s[0] {
					toUpdate[existingRecords[0].ID] = ipv4s
				}
			} else {
				log.Warn().Str("hostname", hostname).
					Str("recordName", recordName).
					Msg("known TODO details")
				return fmt.Errorf("known TODO: compute safe patches for 100.0.0.0/8 entries")
			}
		} else {
			// requires
			toCreate[recordName] = ipv4s
		}
	}
	log.Info().
		Interface("toUpdate", toUpdate).
		Interface("toCreate", toCreate).
		Interface("toDelete", toDelete).
		Msg("queued Cloudflare changes")
	// update 'em
	// ...or just leave because it's a dry run!
	if opts.DryRun {
		return nil
	}
	cfMutateRecordURL := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records", cloudflareZone)
	for name, ipv4s := range toCreate {
		for _, ipv4 := range ipv4s {
			body, err := json.Marshal(map[string]interface{}{
				"type":    "A",
				"name":    name,
				"content": ipv4,
				"ttl":     1,
				"proxied": false,
			})
			log.Debug().Str("body", string(body)).Msg("updating record")
			if err != nil {
				return fmt.Errorf("error creating DNS POST request body: %s", err)
			}
			request, err := http.NewRequest("POST", cfMutateRecordURL, bytes.NewBuffer(body))
			if err != nil {
				return fmt.Errorf("error creating DNS POST request: %s", err)
			}
			request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", cloudflareToken))
			request.Header.Set("Content-Type", "application/json")
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				return fmt.Errorf("error performing Cloudflare record POST: %s", err)
			}
			body, err = ioutil.ReadAll(response.Body)
			if err != nil {
				return fmt.Errorf("error reading Cloudflare record POST: %s", err)
			}
			if response.StatusCode > http.StatusAccepted {
				return fmt.Errorf(">202 response to Cloudflare record POST: %d: %s", response.StatusCode, body)
			}
			log.Debug().Str("body", string(body)).Msg("record POST response")
		}
	}
	// TODO: update records
	// delete records
	for _, recordIDs := range toDelete {
		for _, recordID := range recordIDs {
			url := fmt.Sprintf("%s/%s", cfMutateRecordURL, recordID)
			request, err := http.NewRequest(http.MethodDelete, url, nil)
			if err != nil {
				return fmt.Errorf("error creating DNS DELETE request: %s", err)
			}
			request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", cloudflareToken))
			request.Header.Set("Content-Type", "application/json")
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				return fmt.Errorf("error performing Cloudflare record DELETE: %s", err)
			}
			body, err = ioutil.ReadAll(response.Body)
			if err != nil {
				return fmt.Errorf("error reading Cloudflare record DELETE: %s", err)
			}
			if response.StatusCode > http.StatusAccepted {
				return fmt.Errorf(">202 response to Cloudflare record DELETE: %d: %s", response.StatusCode, body)
			}
			log.Debug().Str("body", string(body)).Msg("record POST response")
		}
	}
	return nil
}
