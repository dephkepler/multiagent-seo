import { twMerge } from 'tailwind-merge'

export function cx(...classes: Array<string | undefined | false | null>): string {
  return twMerge(classes.filter(Boolean).join(' '))
}
