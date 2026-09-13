# Metasearch

Suchrouter ohne Oberfläche und ohne externe Go-Abhängigkeiten. Eine Browsersuche wird per HTTP 302 direkt zu Google oder einem konfigurierten Bang-Ziel weitergeleitet. Keine Ergebnisaggregation, ausgehenden API-Aufrufe oder Datenbank.

## Start mit Docker Compose

Repository: [BigFabe/metasearch](https://github.com/BigFabe/metasearch).

```sh
cp config.example.json config.json
# public_url und Suchziele in config.json bearbeiten
chmod 644 config.json
docker compose up -d
docker exec metasearch /metasearch -h
```

`public_url` ist die öffentliche HTTPS-Adresse ohne Pfad, etwa `https://search.example.com`. Für einen lokalen Test ist `http://localhost:8080` möglich. Die Konfiguration enthält keine Geheimnisse und muss für Container-UID 65532 lesbar sein. Sie wird schreibgeschützt eingebunden und nicht ins Image kopiert.

Compose verbindet den Container mit dem vorhandenen externen Docker-Netzwerk `lsio`.
Der Dienst lauscht darin als `metasearch:8558`; es wird kein Port am Host
veröffentlicht. Nginx Proxy Manager übernimmt HTTPS und verwendet als Forward Hostname
`metasearch` sowie als Forward Port `8558`.

Nach Änderungen die Konfiguration neu einbinden und laden:

```sh
docker compose up -d --force-recreate
```

`compose.yaml` verwendet das veröffentlichte Image
`ghcr.io/bigfabe/metasearch:latest` und lädt beim Start die aktuelle Version dieses
Tags. Ein lokaler Build ist nicht erforderlich. Das erneute Erstellen berücksichtigt
auch Editoren, die Dateien per Umbenennung ersetzen. Zum Stoppen: `docker compose down`.

## Release-Images

Jeder veröffentlichte GitHub-Release startet den Workflow **Publish release image**.
Er prüft den Code mit `go test -race` und `go vet` und veröffentlicht anschließend
ein Image für `linux/amd64` und `linux/arm64` in der GitHub Container Registry:

```text
ghcr.io/bigfabe/metasearch:v0.1.0
ghcr.io/bigfabe/metasearch:latest
```

Das Versions-Tag entspricht dem Release-Tag. Reguläre Releases aktualisieren außerdem
`latest`; Vorabversionen erhalten nur ihr Versions-Tag. Entwürfe starten keinen Build.
Der Workflow verwendet das automatisch bereitgestellte `GITHUB_TOKEN`; zusätzliche
Registry-Secrets sind nicht erforderlich. Actions sind auf Commit-SHAs festgelegt.

Für einen reproduzierbar festgelegten Stand kann in `compose.yaml` statt `latest`
ein Release-Tag wie `ghcr.io/bigfabe/metasearch:v0.1.0` eingetragen werden. Updates
des festgelegten Tags erfolgen bewusst durch Anpassen dieser Zeile. Zum Aktualisieren:

```sh
docker compose pull
docker compose up -d
```

Weitere Releases über die GitHub-Releases-Seite oder per CLI veröffentlichen:

```sh
git tag v0.1.1
git push origin v0.1.1
gh release create v0.1.1 --verify-tag --title v0.1.1 --generate-notes
```

Der Workflow baut den Stand des Release-Tags. Nach der Veröffentlichung den
Workflow-Erfolg prüfen, bevor das neue Image eingesetzt wird.

## Konfiguration und Bangs

```json
{
  "public_url": "https://search.example.com",
  "default_search": "https://www.google.com/search?q={query}",
  "bangs_without_exclamation": false,
  "bangs": {
    "g": "https://www.google.com/search?q={query}",
    "gh": "https://github.com/search?q={query}",
    "w": "https://de.wikipedia.org/w/index.php?search={query}"
  }
}
```

`default_search` legt die Standardsuche fest. Jeder Eintrag in `bangs` verbindet ein Kürzel ohne `!` mit einer absoluten HTTP(S)-URL. `{query}` muss wörtlich im URL-Pfad oder in einem Query-Parameterwert stehen; es wird passend für die jeweilige Stelle URL-kodiert. Zusätzliche feste Parameter sind möglich. Kürzel erlauben ASCII-Buchstaben, Ziffern, `_` und `-` und unterscheiden Groß- und Kleinschreibung. Eine leere oder weggelassene Bang-Zuordnung ist erlaubt.

| Eingabe | Verhalten |
| --- | --- |
| `linux` | Standardsuche |
| `!gh linux` oder `linux !gh` | GitHub-Suche nach `linux` |
| `hello !gh world` | GitHub-Suche; nur das Bang-Wort wird entfernt |
| `!w !gh linux` | Wikipedia-Suche nach `!gh linux` |
| `!unbekannt linux` | Vollständiger Text an die Standardsuche |
| `!GH linux` oder `hello!gh` | Normaler Suchtext |
| `!gh` | GitHub-Such-URL mit leerem Suchparameter |

Bangs sind durch Leerraum getrennte Wörter, einschließlich Unicode-Leerraum. Der erste bekannte Bang gewinnt. Der übrige Text bleibt erhalten; nach Bang-Entfernung wird Leerraum an den Rändern entfernt. Leere Suchanfragen, fehlerhafte URL-Kodierung und mehrfache `q`-Parameter ergeben HTTP 400. Ungültige Konfiguration verhindert den Start mit einer Meldung ohne Konfigurationswerte.

Mit `"bangs_without_exclamation": true` werden konfigurierte Kürzel auch ohne `!` erkannt: `gh linux`, `linux gh` und `hello gh world` verwenden die GitHub-Suche. Die Schreibweise `!gh` funktioniert weiterhin. Auch bei gemischten Schreibweisen gewinnt das erste bekannte Kürzel. Damit werden normale Wörter, die einem konfigurierten Kürzel entsprechen, ebenfalls als Bang behandelt. Bei `false` oder weggelassener Option bleibt `!` erforderlich.

## Browser einrichten

Such-URL: `https://search.example.com/search?q=%s` (Domain ersetzen).

- **Firefox:** Einstellungen → Suche → Suchmaschinen/Suchmaschinen-Schlüsselwörter → Hinzufügen. Name `Metasearch`, obige Such-URL eintragen, anschließend als Standardsuchmaschine auswählen. Die manuelle Einrichtung wird ab Firefox 140 unterstützt; Bezeichnungen unterscheiden sich nach Version. Siehe [Mozilla-Anleitung](https://support.mozilla.org/en-US/kb/add-or-remove-search-engine-firefox).
- **Chromium / Chrome / Brave:** Einstellungen → Suchmaschine → Suchmaschinen und Websitesuche verwalten → Hinzufügen. Name `Metasearch`, Kürzel `meta`, obige URL eintragen; anschließend im Menü als Standard festlegen. Siehe [Chrome-Anleitung](https://support.google.com/chrome/answer/95426).

`/opensearch.xml` liefert zusätzlich eine OpenSearch-Beschreibung. `/` verweist im HTTP-Link-Header darauf. Die Erkennung dieses Headers ist browserabhängig; die manuelle Einrichtung benötigt keine automatische Erkennung. Eine grafische Oberfläche oder Browser-Erweiterung ist nicht erforderlich.

## Datenschutz

Der Dienst speichert weder Suchanfragen noch IP-Adressen, Request-URLs, Header oder Suchhistorien. Es gibt keine Access-Logs, Cookies, Analytics oder requestbezogenen Metriken. Die Konfiguration wird einmal beim Start gelesen; der Suchpfad arbeitet ausschließlich im Speicher und greift nicht auf Dateien oder externe Dienste zu. Weiterleitungen enthalten keinen Antworttext mit Suchdaten.

Betriebslogs enthalten nur Start/Stopp und feste technische Fehlermeldungen. Panic-Werte und die möglicherweise anfragebezogenen Diagnosen von `net/http` werden nicht ausgegeben. Alle vom Handler erzeugten Antworten tragen `Cache-Control: no-store` und `Referrer-Policy: no-referrer`, auch Fehler. Protokollfehler, die Go vor dem Handler abweist, können diese Header nicht erhalten; die Reverse-Proxy-Konfiguration ergänzt sie auch für solche HTTP-Fehler.

[deploy/nginx.conf](deploy/nginx.conf) ist eine vollständige Beispielkonfiguration für einen **dedizierten Nginx**, mit deaktivierten Access- und Fehlerlogs, Cache, Proxy-Speicherung und temporärer Proxy-Pufferung. Domain und Zertifikatspfade müssen angepasst werden. Der HTTPS-Proxy ergänzt die Datenschutz-Header auch bei eigenen Fehlerantworten. HTTP-Anfragen werden abgewiesen; im Browser unbedingt die HTTPS-URL verwenden.

Bei Übernahme in einen bestehenden Proxy müssen auch dessen übergeordnete Logs berücksichtigt werden: Fehler vor der virtuellen Hostauswahl können im globalen Fehlerlog landen. Deshalb deaktiviert das vollständige Beispiel dieses ebenfalls. Keine zusätzlichen Request-Logger, WAF-Traces oder CDN-Logs vorschalten. Grundlagen: [Nginx Access-Logs](https://nginx.org/en/docs/http/ngx_http_log_module.html#access_log), [Nginx Fehlerlogs](https://nginx.org/en/docs/ngx_core_module.html#error_log).

Die Anwendung hält Anfragen nur während der Verarbeitung im Arbeitsspeicher; Go garantiert keine sofortige Löschung freigegebenen Speichers. Für Schutz vor Speicherabbildern sind auch Host-Swap, Hibernation, Backups und Debugger relevant. Compose deaktiviert Core-Dumps und verwendet ein schreibgeschütztes Dateisystem; für strikte Vermeidung unverschlüsselter Speicherauslagerung muss der Host entsprechend konfiguriert sein.

Die Ziel-Suchmaschine erhält den Suchtext und die Verbindung des Browsers zwangsläufig. Deren Speicherung sowie Browserverlauf und Browser-Synchronisation kontrolliert dieser Dienst nicht.

## Schnittstellen

| Pfad | Antwort |
| --- | --- |
| `/search?q=…` | 302 mit `Location`, bei ungültiger Anfrage 400 |
| `/` | Kurzer Klartexthinweis und OpenSearch-Link-Header |
| `/opensearch.xml` | OpenSearch-XML |
| `/healthz` | 200, `ok` |

GET und HEAD werden unterstützt; andere Methoden liefern 405, unbekannte Pfade 404. Der Server begrenzt Header auf etwa 16 KiB (zuzüglich Go-internem Lesepuffer) und verwendet Header-/Lese-/Schreib-/Idle-Timeouts von 5/10/10/60 Sekunden. SIGTERM/SIGINT ermöglichen bis zu fünf Sekunden für laufende Anfragen.

## Lokal bauen und prüfen

Go 1.26 oder neuer; das Container-Build verwendet Go 1.27.1. Keine weiteren Go-Pakete nötig.

```sh
go test -race ./...
go vet ./...
go test -run '^$' -bench . -benchmem
CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o metasearch .
GOTRACEBACK=none ./metasearch -config config.example.json -listen 127.0.0.1:8080
```

Tests prüfen Suchziele, Unicode/URL-Kodierung, ungültige Konfiguration, Statuscodes, Datenschutz-Header, XML und leere Logs bei normalen und fehlerhaften Anfragen einschließlich HTTP-Parserfehlern. Benchmarks messen lokale Auflösung und Handler, nicht Netzwerk- oder Suchmaschinenlatenz.

Container-Smoke-Test nach dem Start:

```sh
docker run --rm --network lsio docker.io/curlimages/curl:latest --fail 'http://metasearch:8558/healthz'
docker run --rm --network lsio docker.io/curlimages/curl:latest -sS -D - -o /dev/null 'http://metasearch:8558/search?q=%21gh+linux'
docker compose logs
```

Erwartet: `ok`, HTTP 302 mit `Location: https://github.com/search?q=linux`, Datenschutz-Header und ausschließlich technische Betriebslogs. `curl` ohne `-L` verwenden, damit keine Anfrage an die Ziel-Suchmaschine gesendet wird.
