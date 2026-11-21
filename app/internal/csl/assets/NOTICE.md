# Third-party notices — litkit CSL runtime

本目录中的资源随 litkit 二进制分发，来源与许可证如下。

## citeproc-js

- 文件：`citeproc.min.js`（citeproc-js 1.4.61）
- 版权：Copyright (c) 2009-2019 Frank Bennett
- 许可：Common Public Attribution License 1.0 (CPAL) **或** GNU Affero General Public License 3.0 (AGPL)，二者择一
- 上游：https://github.com/Juris-M/citeproc-js
- Go 封装参考：https://github.com/sett17/citeproc-js-go（MIT）

litkit 未修改 citeproc-js 源码，仅原样嵌入，并选择 **CPAL 1.0** 作为使用许可（AGPL 与项目 Apache-2.0 不兼容）。
分发时须随附本目录 `CITEPROC-LICENSE.txt`、CPAL 1.0 全文与上游源码地址。CPAL 全文：
https://opensource.org/license/cpal-1-0

> 合规提示：CPAL 为文件级 copyleft。发布前应由项目所有者确认所选许可与分发方式兼容。
> 当前 goreleaser 配置已将 `NOTICE.md`、`CITEPROC-LICENSE.txt` 与 `../LICENSE` 放入归档；
> 如使用裸二进制直传，需在 Release 页面额外附上本 NOTICE 与 citeproc-js 许可。

## CSL 样式

| 文件 | 来源 | 许可 |
|---|---|---|
| `gb7714-2025.csl` | https://github.com/zotero-chinese/styles（GB/T 7714—2025 顺序编码，双语） | CC BY-SA 3.0 |
| `ieee.csl` | https://github.com/citation-style-language/styles（IEEE） | CC BY-SA 3.0 |
| `apa.csl` | litkit 紧凑实现，参照 APA 7th 官方指南 | 随 litkit（Apache-2.0） |

GB/T 与 IEEE 样式未修改；如修改，需按 CC BY-SA 3.0 以相同许可发布修改版本并注明来源。

## CSL locale

| 文件 | 来源 | 许可 |
|---|---|---|
| `locales-zh-CN.xml` | https://github.com/citation-style-language/locales | CC BY-SA 3.0 |
| `locales-en-US.xml` | https://github.com/citation-style-language/locales | CC BY-SA 3.0 |
