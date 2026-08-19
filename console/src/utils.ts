export const formatDate = (date: string | null, includeTime = false) => {
  if (!date) return 'Never expires';
  return new Intl.DateTimeFormat('en-GB', {
    day: '2-digit',
    month: 'short',
    year: 'numeric',
    ...(includeTime ? { hour: '2-digit', minute: '2-digit' } : {}),
  }).format(new Date(date));
};
