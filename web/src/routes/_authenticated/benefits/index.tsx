import { createFileRoute } from '@tanstack/react-router'

import { UserBenefits } from '@/features/benefits'

export const Route = createFileRoute('/_authenticated/benefits/')({
  component: UserBenefits,
})
