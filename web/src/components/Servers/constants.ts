import { Cloud, Key } from 'lucide-react';

export interface ProviderConfig {
  id: string;
  name: string;
  icon: typeof Cloud;
  desc: string;
  bgColor: string;
  textColor: string;
  badgeColor: string;
  letter: string;
}

export const providers: ProviderConfig[] = [
  {
    id: 'hetzner',
    name: 'Hetzner Cloud',
    icon: Cloud,
    desc: 'Provision new server',
    bgColor: 'bg-red-500/10',
    textColor: 'text-red-500',
    badgeColor: 'bg-red-500/10 text-red-600 border-red-500/20 dark:text-red-400',
    letter: 'HZ',
  },
  {
    id: 'digitalocean',
    name: 'DigitalOcean',
    icon: Cloud,
    desc: 'Provision new server',
    bgColor: 'bg-blue-500/10',
    textColor: 'text-blue-500',
    badgeColor: 'bg-blue-500/10 text-blue-600 border-blue-500/20 dark:text-blue-400',
    letter: 'DO',
  },
  {
    id: 'vultr',
    name: 'Vultr',
    icon: Cloud,
    desc: 'Provision new server',
    bgColor: 'bg-purple-500/10',
    textColor: 'text-purple-500',
    badgeColor: 'bg-purple-500/10 text-purple-600 border-purple-500/20 dark:text-purple-400',
    letter: 'VL',
  },
  {
    id: 'custom',
    name: 'Custom SSH',
    icon: Key,
    desc: 'Connect existing server',
    bgColor: 'bg-muted',
    textColor: 'text-muted-foreground',
    badgeColor: 'bg-muted text-muted-foreground',
    letter: 'SSH',
  },
];

export function getProviderConfig(providerId: string): ProviderConfig {
  return providers.find((p) => p.id === providerId) || providers[3];
}

export const regions: Record<string, { id: string; name: string }[]> = {
  hetzner: [
    { id: 'fsn1', name: 'Falkenstein' },
    { id: 'nbg1', name: 'Nuremberg' },
    { id: 'hel1', name: 'Helsinki' },
    { id: 'ash', name: 'Ashburn' },
  ],
  digitalocean: [
    { id: 'nyc1', name: 'New York 1' },
    { id: 'sfo3', name: 'San Francisco 3' },
    { id: 'ams3', name: 'Amsterdam 3' },
    { id: 'fra1', name: 'Frankfurt 1' },
  ],
  vultr: [
    { id: 'ewr', name: 'New Jersey' },
    { id: 'lax', name: 'Los Angeles' },
    { id: 'fra', name: 'Frankfurt' },
    { id: 'nrt', name: 'Tokyo' },
  ],
};

export const sizes = [
  { id: 'small', name: 'Small', desc: '2 vCPU / 2 GB RAM / 40 GB' },
  { id: 'medium', name: 'Medium', desc: '4 vCPU / 8 GB RAM / 80 GB' },
  { id: 'large', name: 'Large', desc: '8 vCPU / 16 GB RAM / 160 GB' },
  { id: 'xlarge', name: 'X-Large', desc: '16 vCPU / 32 GB RAM / 320 GB' },
];