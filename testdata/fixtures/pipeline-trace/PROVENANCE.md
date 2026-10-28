# Canonical trace fixture provenance

Copied without modification from obey-voice commit 15c572c1c7331515fb937f58846f7b2689b97145, testdata/pipeline-trace. Original six fixtures from completed campaign design WI-b04133 at cba388a4 remain byte-identical; background-job.jsonl is the additive integration fixture with typed source job_id/causal parent and deliberately opaque span IDs. These are synthetic contract fixtures, never real voice/performance evidence.

SHA256:

```
485dadf44795cdedd00be5d4eef988955e21009f70a37fa3ecfcb57332f381f6  background-job.jsonl
567241aab9cacb8170482fbfd36675de3a68a7576e18c419e02868be63f64285  fallback.jsonl
eb56cb3e3a99552c8a90888cec6289c02889b1fffc3536c4e700ed94fbdcaf16  incomplete.jsonl
6fe9e2210b4db15444e28a41f969c5744f22a9b3549026be61aa3dde60f89a14  late-attach.jsonl
d2e5a0d30f025e77bbc636ce0a47365f01ef71158e25e83f70da1d579f1b57f8  overlap-interrupted.jsonl
33419aa937e23269e68563c7d50a5fe838537eed05610dfd0698ae4712e73fcc  partial.jsonl
36b14db3ca5ad90fdf9e572901fa680c412a34c8f6a844cfbfa2e943031e5e36  unknown.jsonl
```
