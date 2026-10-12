# Transfer size limits

Investigated on 2026-10-11 against `disk.pku.edu.cn`, using the saved account and read-only analysis of `北大网盘2.0_All_Linux_x64-7.0.6.3-20250716-Terminator-281.deb` (AnyShare 7.0.6-3-281). The Debian package and its ASAR were unpacked only under `/tmp`. The client and package scripts were not executed or installed. Package SHA-256: `d90386c33065d8932380f6c530965c09bf57ea561833b6c75fe5101b698da26e`.

## Observed limits

| Path | Evidence | Effect |
| --- | --- | --- |
| Single upload (`osbeginupload`) | Initiation accepts 5 GiB; 5 GiB + 1 byte returns HTTP 400/code `400000000`, identifying `length` as invalid. | Files over 5 GiB must use multipart. |
| Multipart (`osoption`) | `partminsize=20971520`, `partmaxsize=5368709120`, `partmaxnum=10000`. | Parts are 20 MiB–5 GiB, with at most 10,000 parts. Reported multipart capacity is 53,687,091,200,000 bytes (about 48.8 TiB). |
| Previous client implementation | 16 MiB parts and a fixed 10,000-part rejection. | Artificial upload ceiling of 156.25 GiB, removed by automatic part growth. |
| Direct file download (`osdownload`) | Streams a signed object-storage GET and supports byte ranges. A complete 20 GiB download succeeded. | No whole-file cap found in this path; validated through 20 GiB. |
| Mounted cache | Reads and writes share a 10 GiB local cache budget by default; clean data is evicted before pending writes. | A writable file must fit in the budget; change `--cache-limit` or set it to `0`. Reads fetch small files whole or bounded windows of large files and have no whole-file size cap. Unmounted transfers do not use this budget. |
| Folder ZIP / remote extraction | `/open-doc/v1/file-download` and `file-unarchiver` work on PKU. The official bundle has archive download and extraction size-limit errors; their exact deployed thresholds were not tested. | Archive modes can have lower limits than direct transfers. Omit archive flags to download files directly. |
| Operation duration | `--io-timeout` defaults to 10 minutes per operation, including a whole directory transfer. | Increase it for long transfers. This is a time limit, not a byte limit. |

The multipart capacity is the server's configuration, not proof that the account can store a file of that size. Quotas, storage capacity, file policies, and object-storage constraints still apply.

## Official client evidence

The unpacked `resources/app.asar` contains `build/static/js/main.49186747.chunk.js`. The bundled upload queue's `calculatePartSize` calls `POST /efast/v1/file/osoption`, starts from a preferred 20 MiB part size (configurable by the client), clamps it to the server's bounds, and raises it to `ceil(fileSize / partmaxnum)` when needed. It rejects a file larger than `partmaxsize * partmaxnum`.

`afterAddFile` chooses `osbeginupload` for small files and `initPartsUpload`/`osinitmultiupload` for larger files. Multipart data goes to the signed `osuploadpart` URLs, with each ETag and length recorded in `partinfo`; `oscompleteupload` returns completion XML and a signed request, followed by `osendupload` to publish. The inspected JavaScript transfer queue and these live API calls provide the protocol evidence; the package's native sync integration also refers to external Windows/macOS components.

The official download manager uses `/open-doc/v1/file-download` and ranged streaming; the bundle also has direct `/efast/v1/file/osdownload` calls. Its 2 GB error messages occur alongside preview, conversion, and batch-download errors. Those messages do not establish a 2 GiB cap on the direct object-storage path.

Administrator size policies are separate: both upload branches and download authorization handle `403002070`, including `detail.file_limit_size`. Multipart removes the single-request endpoint limit; it does not guarantee exemption from those policies.

## Implementation and validation

Uploads now negotiate the server limits before creating an upload. The selected part size is:

```text
max(ceil(fileSize / partmaxnum), clamp(preferredPartSize, partminsize, partmaxsize))
```

The client rejects files exceeding the reported multipart capacity before creating a remote entry. Small and empty files retain the single-request flow. All interfaces share this backend. Server size-policy errors include the returned byte limit and become `EFBIG` in a mount.

Downloads stream 512 MiB ranges with a fresh signed URL for each range, preserving the selected revision and checking the range and exact byte count. The 512 MiB value bounds each request; it does not bound the file size.

`upload --fast` uses the official instant-upload shortcut for new files. It
posts an MD5 of the first 204800 bytes and the file length to `predupload`. If
the service reports a match, the client reads the local file once with a
256 KiB buffer, calculates full MD5 and CRC32, and posts them to `dupload` with
the destination ID, name, duplicate policy and millisecond `client_mtime`.
The server reuses its existing object and no file bytes are sent. A miss falls
back to the ordinary uploader. Existing files, including `--overwrite`, keep
the revision-checked ordinary path. The shortcut is optional because a newly
uploaded object may not be visible to the server's deduplication index
immediately.

Validation includes logical large-file planning without allocation (including maximum `int64` values), the exact server multipart capacity and the byte above it, rejection before remote creation, automatically enlarged multipart chunks and completion metadata, invalid or truncated download ranges, and very large read offsets.

A live 21 MiB multipart round trip passed. A full 5 GiB + 1 byte round trip with 64 MiB upload parts also passed: upload/publish took 1m42s, download took 50s, and SHA-256 matched (`e50821214328d309c11354522317a039e49e9bf738f87dabf511278af7b17b7b`). The upload source generated bytes on demand and the download streamed into a hash; no large local file or whole-file memory buffer was used. The remote test folders were deleted.

The larger live acceptance transferred **20 GiB (21,474,836,480 bytes)** using 320 upload parts of 64 MiB and 40 download ranges of 512 MiB. Upload and publication took **6m22s**; download took **3m19s**. Remote size matched the source size, and the downloaded SHA-256 matched the independently computed source hash: `945f8185a880b6e72b39248f50e198140742b27c7422d665680842a995ba9e69`.

This test generated bytes on demand, including markers beyond 4 GiB, and streamed the download directly into SHA-256. Sampled resident memory stayed around **17–18 MiB**; it created no local upload/download data file. The temporary remote folder was deleted after verification. This exercised the shared AnyShare transfer backend; the local-file and mount paths are covered by the repository checks.

`make test` (including the race detector), `make check`, and `make build` passed.
