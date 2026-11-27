# SharePoint sample documents

The generator creates a synthetic Office corpus for local testing of the
SharePoint adapter. It includes Word, PowerPoint, and Excel-shaped documents
with headings, slides, tables, and fictional policy content.

```bash
go run gen.go
```

Generated files are written to the ignored `out/` directory. Do not upload
real customer or tenant documents while experimenting with this example.

