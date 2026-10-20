import Icon from './Icon'

// An error that blocks something: what happened and what to do next.
export default function Alert({ message }: { message: string }) {
  return (
    <p role="alert" className="cn-alert">
      <Icon name="alert" />
      <span>{message}</span>
    </p>
  )
}
