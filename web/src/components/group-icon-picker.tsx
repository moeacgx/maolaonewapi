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
import { Check, ChevronsUpDown } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { filterGroupIconOptions } from '@/lib/group-icons'
import { getLobeIcon } from '@/lib/lobe-icon'
import { cn } from '@/lib/utils'
import { Button } from './ui/button'
import {
  Command,
  CommandEmpty,
  CommandInput,
  CommandItem,
  CommandList,
} from './ui/command'
import { Popover, PopoverContent, PopoverTrigger } from './ui/popover'

type GroupIconPickerProps = {
  value?: string
  onChange: (value: string) => void
  disabled?: boolean
}

export function GroupIconPicker({
  value,
  onChange,
  disabled = false,
}: GroupIconPickerProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const options = useMemo(() => filterGroupIconOptions(query), [query])
  const selected = value || 'Layers'
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        render={
          <Button
            type='button'
            variant='outline'
            disabled={disabled}
            className='h-9 w-full justify-between'
          />
        }
      >
        <span className='flex items-center gap-2 truncate'>
          {getLobeIcon(selected, 18)}
          <span className='truncate'>{selected}</span>
        </span>
        <ChevronsUpDown className='size-4 opacity-50' />
      </PopoverTrigger>
      <PopoverContent className='w-72 p-0'>
        <Command shouldFilter={false}>
          <CommandInput placeholder={t('Search icons...')} value={query} onValueChange={setQuery} />
          <CommandList className='max-h-72'>
            <CommandEmpty>{t('No icon found.')}</CommandEmpty>
            {options.map((option) => (
              <CommandItem
                key={option.value}
                value={option.value}
                onSelect={() => {
                  onChange(option.value)
                  setOpen(false)
                }}
                className='gap-2'
              >
                {getLobeIcon(option.value, 20)}
                <span className='flex-1'>{option.label}</span>
                <Check
                  className={cn(
                    'size-4',
                    selected === option.value
                      ? 'opacity-100'
                      : 'opacity-0'
                  )}
                />
              </CommandItem>
            ))}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}
