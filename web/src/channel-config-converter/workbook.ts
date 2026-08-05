/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import {
  TextReader,
  TextWriter,
  Uint8ArrayReader,
  Uint8ArrayWriter,
  ZipReader,
  ZipWriter,
} from '@zip.js/zip.js'
/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import ExcelJS from 'exceljs'

import type { CellSnapshot, CellValue, WorkbookSnapshot } from './types'

export const V1_HEADERS = {
  使用说明: [
    '工作表',
    '用途',
    '主键',
    '主要输入',
    '主要输出',
    '是否导入系统',
    '维护频率',
    '备注',
  ],
  参数: ['参数代码', '值', '单位', '说明'],
  渠道: [
    '渠道代码',
    '渠道名称',
    '价格页',
    '默认币种',
    '充值兑换比例',
    '手续费率',
    '计费倍率',
    '付费模式',
    '状态',
    '并发数',
    '严格成本校验',
    '采集日期',
    '来源ID',
    '备注',
    '校验',
  ],
  模型SKU: [
    'SKU代码',
    '模型',
    '版本',
    '分辨率档位',
    '输出宽',
    '输出高',
    '帧率',
    '最小时长秒',
    '最大时长秒',
    '比例',
    '支持视频输入',
    '支持真人脸',
    '支持超分',
    '计量方法',
    '状态',
    '来源ID',
    '备注',
    '校验',
  ],
  官方售价: [
    '售价ID',
    '客户端模型',
    'SKU代码',
    '定价场景',
    '计费模式',
    '币种',
    '原币/1M',
    '输出宽',
    '输出高',
    '帧率',
    'Token/基准秒',
    '原币/基准秒',
    'USD/1M',
    'USD/基准秒',
    '生效时间',
    '失效时间',
    '状态',
    '来源ID',
    '备注',
    '校验',
  ],
  渠道成本: [
    '成本规则ID',
    '渠道代码',
    '上游模型',
    'SKU代码',
    '定价场景',
    '成本模式',
    'Token子模式',
    '计量来源',
    'Token字段',
    '计费事件',
    '币种',
    '原币按次',
    '原币/秒',
    '原币/1M',
    '原币基础单价',
    '计费倍率',
    '采购折扣',
    '充值兑换比例',
    '手续费率',
    '原币兑USD',
    '标准USD单价',
    '计价单位',
    '生效时间',
    '失效时间',
    '状态',
    '来源ID',
    '备注',
    '校验',
  ],
  模型映射: [
    '映射ID',
    '客户端模型',
    '渠道代码',
    '上游模型',
    'SKU代码',
    '默认定价场景',
    '启用',
    '最小时长秒',
    '最大时长秒',
    '可用时长秒',
    '生效时间',
    '失效时间',
    '来源ID',
    '备注',
    '校验',
  ],
  利润测算: [
    '场景ID',
    '官方售价ID',
    '成本规则ID',
    '分组倍率',
    '输入视频秒',
    '输出视频秒',
    'SKU代码',
    '定价场景',
    '输出宽',
    '输出高',
    '帧率',
    '估算Token',
    '成本模式',
    '标准USD单价',
    '官方售价USD',
    '渠道成本USD',
    '用户收入USD',
    '毛利润USD',
    '毛利率',
    '成本状态',
    '说明',
  ],
  来源: [
    '来源ID',
    '项目',
    '数值/范围',
    '单位',
    '截止日期',
    '来源类型',
    '来源名称',
    '引用位置',
    '负责人',
    '备注',
    '访问日期',
    '校验',
  ],
  校验: ['检查项', '错误数', '状态', '修复位置', '说明'],
} as const

export const V2_HEADERS = {
  渠道: [
    'channel_ref',
    'display_name',
    'currency',
    'recharge_ratio',
    'fee_rate',
    'billing_multiplier',
    'status_proposal',
    'source_ref',
    'note',
  ],
  渠道线路: [
    'line_ref',
    'channel_ref',
    'display_name',
    'provider_type_hint',
    'region',
    'protocol',
    'supports_real_person',
    'status_proposal',
    'source_ref',
    'note',
  ],
  模型SKU: [
    'sku_ref',
    'canonical_model',
    'resolution',
    'output_width',
    'output_height',
    'frame_rate',
    'duration_min',
    'duration_max',
    'source_ref',
  ],
  官方售价: [
    'sale_ref',
    'client_model',
    'sku_ref',
    'scenario',
    'billing_mode',
    'currency',
    'native_unit_price',
    'status_proposal',
    'source_ref',
  ],
  渠道成本: [
    'cost_rule_ref',
    'line_ref',
    'upstream_model',
    'sku_ref',
    'scenario',
    'cost_mode',
    'cost_variant_key',
    'route_target_ref',
    'currency',
    'currency_to_usd_rate',
    'billing_multiplier',
    'purchase_discount_ratio',
    'recharge_exchange_ratio',
    'fee_rate',
    'native_unit_price',
    'normalized_usd_unit_price',
    'status_proposal',
    'source_ref',
    'note',
  ],
  路由目标: [
    'route_target_ref',
    'canonical_model',
    'client_model',
    'merge_mode',
    'line_ref',
    'upstream_model',
    'sku_ref',
    'cost_variant_key',
    'output_resolutions',
    'duration_min',
    'duration_max',
    'reference_min_images',
    'reference_min_videos',
    'reference_min_audios',
    'reference_max_images',
    'reference_max_videos',
    'reference_max_audios',
    'supports_real_person',
    'priority',
    'enabled',
  ],
  模型映射: [
    'mapping_ref',
    'client_model',
    'line_ref',
    'upstream_model',
    'sku_ref',
    'route_target_ref',
    'source_ref',
    'note',
  ],
  来源: ['source_ref', 'name', 'url', 'collected_at', 'note'],
} as const

function snapshotCellValue(value: unknown, fallback: string): CellValue {
  if (
    value === null ||
    typeof value === 'boolean' ||
    typeof value === 'number' ||
    typeof value === 'string' ||
    value instanceof Date
  ) {
    return value
  }
  if (typeof value === 'object' && value !== null && 'text' in value) {
    const text = value.text
    if (typeof text === 'string') {
      return text
    }
  }
  return fallback === '' ? null : fallback
}

function snapshotCell(cell: ExcelJS.Cell): CellSnapshot {
  const rawValue = cell.value
  if (
    typeof rawValue === 'object' &&
    rawValue !== null &&
    'formula' in rawValue
  ) {
    const formula =
      typeof rawValue.formula === 'string' ? rawValue.formula : null
    const formulaResult = snapshotCellValue(rawValue.result, cell.text)
    return {
      value: formulaResult,
      formula,
      formulaResult,
    }
  }
  const value = snapshotCellValue(rawValue, cell.text)
  return { value, formula: null, formulaResult: null }
}

function normalizeArtifactToolXml(fileName: string, value: string): string {
  const normalized = value.replaceAll('<x:', '<').replaceAll('</x:', '</')
  if (fileName.startsWith('xl/worksheets/_rels/')) {
    return normalized.replaceAll(
      /<Relationship\b(?=[^>]*Type="[^"]*\/(?:comments|threadedComment)")[^>]*\/>/gi,
      ''
    )
  }
  if (!fileName.startsWith('xl/worksheets/')) {
    return normalized
  }
  const tablePartsStart = normalized.indexOf('<tableParts')
  const tablePartsEnd = normalized.indexOf('</tableParts>')
  if (tablePartsStart < 0 || tablePartsEnd < tablePartsStart) {
    return normalized
  }
  return (
    normalized.slice(0, tablePartsStart) + normalized.slice(tablePartsEnd + 13)
  )
}

async function prepareForExcelJs(input: Uint8Array): Promise<Uint8Array> {
  const reader = new ZipReader(new Uint8ArrayReader(input), {
    useWebWorkers: false,
  })
  const writer = new ZipWriter(new Uint8ArrayWriter(), {
    useWebWorkers: false,
  })

  try {
    for (const entry of await reader.getEntries()) {
      if (
        entry.directory ||
        entry.filename.startsWith('xl/tables/') ||
        entry.filename.startsWith('xl/comments') ||
        entry.filename.startsWith('xl/threadedcomments/') ||
        entry.filename.startsWith('xl/persons/')
      ) {
        continue
      }
      if (entry.filename.endsWith('.xml') || entry.filename.endsWith('.rels')) {
        const xml = await entry.getData(new TextWriter())
        await writer.add(
          entry.filename,
          new TextReader(normalizeArtifactToolXml(entry.filename, xml))
        )
        continue
      }
      await writer.add(
        entry.filename,
        new Uint8ArrayReader(await entry.getData(new Uint8ArrayWriter()))
      )
    }
    return await writer.close()
  } finally {
    await reader.close()
  }
}

export async function loadWorkbookSnapshot(
  input: ArrayBuffer | Uint8Array
): Promise<WorkbookSnapshot> {
  const workbook = new ExcelJS.Workbook()
  const source = new Uint8Array(input)
  const prepared = await prepareForExcelJs(source)
  await workbook.xlsx.load(prepared as never)

  return {
    sheets: workbook.worksheets.map((worksheet) => ({
      name: worksheet.name,
      rows: Array.from({ length: worksheet.rowCount }, (_, rowIndex) => {
        const rowNumber = rowIndex + 1
        const row = worksheet.getRow(rowNumber)
        return {
          rowNumber,
          cells: Array.from(
            { length: worksheet.columnCount },
            (_, columnIndex) => snapshotCell(row.getCell(columnIndex + 1))
          ),
        }
      }),
    })),
  }
}
