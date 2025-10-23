package manager

import (
    "encoding/csv"
    "encoding/json"
    "errors"
    "fmt"
    "os"
    "path/filepath"
    "sort"
    "strings"

    libFile "github.com/trustwallet/assets-go-libs/file"
    assetsPath "github.com/trustwallet/assets-go-libs/path"
    "github.com/trustwallet/assets-go-libs/validation/info"
    "github.com/trustwallet/assets/internal/config"

    log "github.com/sirupsen/logrus"
)

type TokenExportRecord struct {
    Chain    string       `json:"chain"`
    Address  string       `json:"address"`
    Name     string       `json:"name,omitempty"`
    Symbol   string       `json:"symbol,omitempty"`
    Decimals int          `json:"decimals,omitempty"`
    Type     string       `json:"type,omitempty"`
    Website  string       `json:"website,omitempty"`
    Explorer string       `json:"explorer,omitempty"`
    Status   string       `json:"status,omitempty"`
    LogoURI  string       `json:"logoURI,omitempty"`
    Tags     []string     `json:"tags,omitempty"`
    Links    []LinkRecord `json:"links,omitempty"`
}

type LinkRecord struct {
    Name string `json:"name,omitempty"`
    URL  string `json:"url,omitempty"`
}

func ExportTokens(outputPath string, format string) error {
    if format != "json" && format != "csv" {
        return fmt.Errorf("unsupported format: %s", format)
    }

    scanRoot := root
    if scanRoot == "" {
        scanRoot = "."
    }
    blockchainsRoot := filepath.Join(scanRoot, "blockchains")

    if _, err := os.Stat(blockchainsRoot); err != nil {
        return fmt.Errorf("blockchains directory not found at %s: %w", blockchainsRoot, err)
    }

    var records []TokenExportRecord

    walkErr := filepath.WalkDir(blockchainsRoot, func(p string, d os.DirEntry, err error) error {
        if err != nil {
            return err
        }
        if d.IsDir() {
            return nil
        }
        if strings.EqualFold(filepath.Base(p), "info.json") {
            rel, err := filepath.Rel(blockchainsRoot, p)
            if err != nil {
                return err
            }
            parts := splitClean(rel)
            if len(parts) != 4 {
                return nil
            }
            chainHandle := parts[0]
            if parts[1] != "assets" {
                return nil
            }
            tokenAddress := parts[2]

            var assetInfo info.AssetModel
            if err := libFile.ReadJSONFile(p, &assetInfo); err != nil {
                log.WithError(err).WithField("path", p).Warn("failed to read token info.json; skipping")
                return nil
            }

            rec := TokenExportRecord{
                Chain:    chainHandle,
                Address:  firstNonEmpty(ptrStr(assetInfo.ID), tokenAddress),
                Name:     ptrStr(assetInfo.Name),
                Symbol:   ptrStr(assetInfo.Symbol),
                Decimals: ptrInt(assetInfo.Decimals),
                Type:     ptrStr(assetInfo.Type),
                Website:  ptrStr(assetInfo.Website),
                Explorer: ptrStr(assetInfo.Explorer),
                Status:   ptrStr(assetInfo.Status),
                LogoURI:  assetsPath.GetAssetLogoURL(config.Default.URLs.AssetsApp, chainHandle, tokenAddress),
                Tags:     assetInfo.Tags,
            }
            for _, l := range assetInfo.Links {
                rec.Links = append(rec.Links, LinkRecord{Name: ptrStr(l.Name), URL: ptrStr(l.URL)})
            }
            records = append(records, rec)
        }
        return nil
    })
    if walkErr != nil {
        return walkErr
    }

    sort.Slice(records, func(i, j int) bool {
        if records[i].Chain != records[j].Chain {
            return records[i].Chain < records[j].Chain
        }
        si := strings.ToLower(records[i].Symbol)
        sj := strings.ToLower(records[j].Symbol)
        if si != sj {
            return si < sj
        }
        return strings.ToLower(records[i].Address) < strings.ToLower(records[j].Address)
    })

    if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
        return err
    }

    switch format {
    case "json":
        return writeJSON(outputPath, records)
    case "csv":
        return writeCSV(outputPath, records)
    default:
        return errors.New("unreachable")
    }
}

func writeJSON(out string, records []TokenExportRecord) error {
    f, err := os.Create(out)
    if err != nil {
        return err
    }
    defer f.Close()
    enc := json.NewEncoder(f)
    enc.SetIndent("", "  ")
    if err := enc.Encode(records); err != nil {
        return err
    }
    log.WithFields(log.Fields{"path": out, "count": len(records)}).Info("tokens exported (json)")
    return nil
}

func writeCSV(out string, records []TokenExportRecord) error {
    f, err := os.Create(out)
    if err != nil {
        return err
    }
    defer f.Close()
    w := csv.NewWriter(f)
    defer w.Flush()
    header := []string{"chain", "address", "name", "symbol", "decimals", "type", "website", "explorer", "status", "logoURI", "tags", "links"}
    if err := w.Write(header); err != nil {
        return err
    }
    for _, r := range records {
        tagsJoined := strings.Join(r.Tags, "|")
        var linkPairs []string
        for _, l := range r.Links {
            name := strings.ReplaceAll(l.Name, "|", "/")
            url := strings.ReplaceAll(l.URL, "|", "/")
            linkPairs = append(linkPairs, fmt.Sprintf("%s:%s", name, url))
        }
        linksJoined := strings.Join(linkPairs, " | ")
        row := []string{r.Chain, r.Address, r.Name, r.Symbol, fmt.Sprintf("%d", r.Decimals), r.Type, r.Website, r.Explorer, r.Status, r.LogoURI, tagsJoined, linksJoined}
        if err := w.Write(row); err != nil {
            return err
        }
    }
    log.WithFields(log.Fields{"path": out, "count": len(records)}).Info("tokens exported (csv)")
    return nil
}

func ptrStr(p *string) string {
    if p == nil {
        return ""
    }
    return *p
}

func ptrInt(p *int) int {
    if p == nil {
        return 0
    }
    return *p
}

func firstNonEmpty(values ...string) string {
    for _, v := range values {
        if strings.TrimSpace(v) != "" {
            return v
        }
    }
    return ""
}

func splitClean(rel string) []string {
    parts := strings.Split(rel, string(os.PathSeparator))
    var out []string
    for _, p := range parts {
        if p != "" {
            out = append(out, p)
        }
    }
    return out
}
