export default function StatusBadge({ online, healthStatus }) {
  let color = 'bg-gray-500'
  let title = 'Offline'

  if (healthStatus === 'online') {
    color = 'bg-green-500'
    title = 'Online'
  } else if (healthStatus === 'degraded') {
    color = 'bg-yellow-500'
    title = 'Degradado (sem internet)'
  } else if (online) {
    color = 'bg-green-500'
    title = 'Online'
  }

  return (
    <span className={`inline-block w-3 h-3 rounded-full ${color}`}
      title={title}
    />
  )
}
