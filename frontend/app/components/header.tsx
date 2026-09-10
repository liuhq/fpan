import { Form } from "react-router"

export default function Header() {
  return (
    <header>
      <Form method="post" action="/logout">
        <button type="submit">Logout</button>
      </Form>
    </header>
  )
}
