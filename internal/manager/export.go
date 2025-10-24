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
            if includeRecord(rec) {
                records = append(records, rec)
            }
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
    // Select columns
    defaultHeader := []string{"chain", "address", "name", "symbol", "decimals", "type", "website", "explorer", "status", "logoURI", "tags", "links"}
    header := defaultHeader
    if strings.TrimSpace(exportColumns) != "" {
        header = splitCSV(exportColumns)
    }
    if err := w.Write(header); err != nil {
        return err
    }
    for _, r := range records {
        row := make([]string, 0, len(header))
        for _, col := range header {
            switch strings.ToLower(strings.TrimSpace(col)) {
            case "chain":
                row = append(row, r.Chain)
            case "address":
                row = append(row, r.Address)
            case "name":
                row = append(row, r.Name)
            case "symbol":
                row = append(row, r.Symbol)
            case "decimals":
                row = append(row, fmt.Sprintf("%d", r.Decimals))
            case "type":
                row = append(row, r.Type)
            case "website":
                row = append(row, r.Website)
            case "explorer":
                row = append(row, r.Explorer)
            case "status":
                row = append(row, r.Status)
            case "logouri":
                row = append(row, r.LogoURI)
            case "tags":
                row = append(row, strings.Join(r.Tags, "|"))
            case "links":
                var linkPairs []string
                for _, l := range r.Links {
                    name := strings.ReplaceAll(l.Name, "|", "/")
                    url := strings.ReplaceAll(l.URL, "|", "/")
                    linkPairs = append(linkPairs, fmt.Sprintf("%s:%s", name, url))
                }
                row = append(row, strings.Join(linkPairs, " | "))
            default:
                row = append(row, "")
            }
        }
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

func splitCSV(s string) []string {
    if strings.TrimSpace(s) == "" {
        return nil
    }
    parts := strings.Split(s, ",")
    var out []string
    for _, p := range parts {
        p = strings.TrimSpace(p)
        if p != "" {
            out = append(out, p)
        }
    }
    return out
}

func includeRecord(r TokenExportRecord) bool {
    // Chains filter
    if strings.TrimSpace(exportChains) != "" {
        allowed := make(map[string]struct{})
        for _, c := range splitCSV(strings.ToLower(exportChains)) {
            allowed[c] = struct{}{}
        }
        if _, ok := allowed[strings.ToLower(r.Chain)]; !ok {
            return false
        }
    }

    // Symbols filter (contains, any)
    if strings.TrimSpace(exportSymbols) != "" {
        symbols := splitCSV(exportSymbols)
        match := false
        for _, s := range symbols {
            if strings.Contains(strings.ToLower(r.Symbol), strings.ToLower(s)) {
                match = true
                break
            }
        }
        if !match {
            return false
        }
    }

    // Name contains
    if strings.TrimSpace(exportNameContains) != "" {
        if !strings.Contains(strings.ToLower(r.Name), strings.ToLower(exportNameContains)) {
            return false
        }
    }

    // Tags (match any)
    if strings.TrimSpace(exportTags) != "" {
        tags := splitCSV(exportTags)
        has := false
        for _, t := range tags {
            for _, rt := range r.Tags {
                if strings.EqualFold(strings.TrimSpace(t), strings.TrimSpace(rt)) {
                    has = true
                    break
                }
            }
            if has {
                break
            }
        }
        if !has {
            return false
        }
    }

    // Type
    if strings.TrimSpace(exportType) != "" && !strings.EqualFold(r.Type, exportType) {
        return false
    }

    // Status
    if strings.TrimSpace(exportStatus) != "" && !strings.EqualFold(r.Status, exportStatus) {
        return false
    }

    return true
}
